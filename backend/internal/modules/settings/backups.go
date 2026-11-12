package settings

// Backups from Settings: the server's backups with their sizes, the nightly
// backup's switch, "Back up now", deleting one, and a short-lived link to
// download one. A backup holds every agency's data on the server, so only
// the agency that runs the server may see them: the one named by
// HP_BACKUP_TENANT, or, when that is not set, the only agency on a server
// with just one.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// BackupSetup says where the server keeps its backups and who may manage them.
type BackupSetup struct {
	Dir            string // HP_BACKUP_DIR; empty: this server keeps none
	OperatorTenant string // HP_BACKUP_TENANT; empty: the only agency, if there is one
	Secret         []byte // signs download links
	Now            func() time.Time
}

// UseBackups gives Settings the server's backups.
func (m *Module) UseBackups(b BackupSetup) {
	if b.Now == nil {
		b.Now = time.Now
	}
	m.service.backups = b
}

// LinkLife is how long a download link works (a download already going finishes).
const LinkLife = 10 * time.Minute

// Backups is what Settings › Backups shows.
type Backups struct {
	Available bool
	Control   backup.Control
	Last      *backup.Last
	List      []backup.Info
	Total     int64
}

// DownloadLink is a short-lived link to one backup.
type DownloadLink struct {
	URL, FileName string
	Size          int64
	ExpiresAt     time.Time
}

func notOperator() error {
	return apperr.New(api.ErrorCodeFORBIDDEN, "Backups are managed by the agency that runs this server.")
}

// operator reports whether p's agency runs this server (and it keeps backups).
func (s *Service) operator(ctx context.Context, tx pgx.Tx, p authz.Principal) (bool, error) {
	b := s.backups
	if b.Dir == "" {
		return false, nil
	}
	if b.OperatorTenant != "" {
		return p.TenantID == b.OperatorTenant, nil
	}
	var n int64
	err := tx.QueryRow(ctx, `SELECT app_tenant_count()`).Scan(&n)
	return n == 1, err
}

// Backups returns the server's backups, newest first, for the agency that runs it.
func (s *Service) Backups(ctx context.Context, p authz.Principal) (Backups, error) {
	var out Backups
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ok, err := s.operator(ctx, tx, p)
		if err != nil || !ok {
			return err
		}
		out, err = s.backupsIn(ctx, tx)
		return err
	})
	return out, err
}

func (s *Service) backupsIn(ctx context.Context, tx pgx.Tx) (Backups, error) {
	out := Backups{Available: true}
	var err error
	if out.Control, err = backup.ReadControl(ctx, tx); err != nil {
		return out, err
	}
	last, ok, err := backup.LastRun(ctx, tx)
	if err != nil {
		return out, err
	}
	if ok {
		out.Last = &last
	}
	if out.List, err = backup.List(s.backups.Dir); err != nil {
		return out, err
	}
	for _, b := range out.List {
		out.Total += b.SizeBytes
	}
	return out, nil
}

// changeBackups runs change for the agency that runs the server, records it,
// and returns the backups after.
func (s *Service) changeBackups(ctx context.Context, p authz.Principal, action string, changes map[string]any, change func(tx pgx.Tx) error) (Backups, error) {
	var out Backups
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ok, err := s.operator(ctx, tx, p)
		if err != nil {
			return err
		}
		if !ok {
			return notOperator()
		}
		// Recorded first: a deleted backup cannot be put back if recording failed after.
		if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "backup", EntityID: "server", Changes: changes}); err != nil {
			return err
		}
		if err := change(tx); err != nil {
			return err
		}
		out, err = s.backupsIn(ctx, tx)
		return err
	})
	return out, err
}

// SetAutoBackup turns the nightly backup on or off.
func (s *Service) SetAutoBackup(ctx context.Context, p authz.Principal, on bool) (Backups, error) {
	return s.changeBackups(ctx, p, "settings.backup_auto_changed", map[string]any{"auto": on}, func(tx pgx.Tx) error {
		return backup.SetAuto(ctx, tx, on)
	})
}

// RequestBackup asks the backup service for a backup now.
func (s *Service) RequestBackup(ctx context.Context, p authz.Principal) (Backups, error) {
	return s.changeBackups(ctx, p, "settings.backup_requested", nil, func(tx pgx.Tx) error {
		return backup.Request(ctx, tx)
	})
}

// DeleteBackup deletes one backup from the server.
func (s *Service) DeleteBackup(ctx context.Context, p authz.Principal, name string) (Backups, error) {
	return s.changeBackups(ctx, p, "settings.backup_deleted", map[string]any{"backup": name}, func(pgx.Tx) error {
		return notFoundBackup(backup.Delete(s.backups.Dir, name))
	})
}

func notFoundBackup(err error) error {
	if errors.Is(err, backup.ErrNotFound) {
		return apperr.NotFound("backup")
	}
	return err
}

// DownloadLink returns a link that downloads one backup for LinkLife.
func (s *Service) DownloadLink(ctx context.Context, p authz.Principal, name string) (DownloadLink, error) {
	var out DownloadLink
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ok, err := s.operator(ctx, tx, p)
		if err != nil {
			return err
		}
		if !ok {
			return notOperator()
		}
		info, err := backup.Stat(s.backups.Dir, name)
		if err != nil {
			return notFoundBackup(err)
		}
		exp := s.backups.Now().Add(LinkLife).Truncate(time.Second)
		expText := strconv.FormatInt(exp.Unix(), 10)
		out = DownloadLink{
			URL:      "/v1/backup-downloads/" + name + "?" + url.Values{"exp": {expText}, "sig": {s.signBackup(name, expText)}}.Encode(),
			FileName: name + ".tar", Size: info.SizeBytes, ExpiresAt: exp,
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "settings.backup_downloaded", EntityType: "backup", EntityID: "server", Changes: map[string]any{"backup": name}})
	})
	return out, err
}

// OpenDownload checks a download link and returns the backup's size; the
// link needs no sign-in, so its signature is the only check.
func (s *Service) OpenDownload(name string, exp int64, sig string) (int64, error) {
	expText := strconv.FormatInt(exp, 10)
	if s.backups.Dir == "" || s.backups.Now().Unix() > exp || !hmac.Equal([]byte(sig), []byte(s.signBackup(name, expText))) {
		return 0, apperr.New(api.ErrorCodeFORBIDDEN, "This download link has expired or is not valid: make a new one in Settings › Backups.")
	}
	size, err := backup.Size(s.backups.Dir, name)
	return size, notFoundBackup(err)
}

func (s *Service) signBackup(name, exp string) string {
	m := hmac.New(sha256.New, s.backups.Secret)
	_, _ = fmt.Fprintf(m, "backup-download\n%s\n%s", name, exp)
	return hex.EncodeToString(m.Sum(nil))
}
