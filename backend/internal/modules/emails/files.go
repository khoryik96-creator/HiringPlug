package emails

// Banners (images a message shows, uploaded once per company) and files
// uploaded to attach to the next email.

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

const maxBannerBytes = 1 << 20

var bannerTypes = []string{"image/png", "image/jpeg", "image/gif"}

// AddBanner stores a banner image (PNG, JPEG or GIF, at most 1 MB).
func (s *Service) AddBanner(ctx context.Context, p authz.Principal, name, fileName string, data []byte) (store.EmailBanner, error) {
	ctype := http.DetectContentType(data)
	var errs apperr.FieldErrors
	switch {
	case len(data) == 0:
		errs.Add("file", "is required")
	case !contains(bannerTypes, ctype):
		errs.Add("file", "must be a PNG, JPEG or GIF image")
	}
	if err := errs.Err(); err != nil {
		return store.EmailBanner{}, err
	}
	if len(data) > maxBannerBytes {
		return store.EmailBanner{}, apperr.New(api.ErrorCodePAYLOADTOOLARGE, "A banner can be at most 1 MB.")
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	}
	if name == "" || name == "." {
		name = "Banner"
	}
	key := p.TenantID + "/ebn_" + newKey()
	if err := s.files.Put(key, data); err != nil {
		return store.EmailBanner{}, err
	}
	var out store.EmailBanner
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).InsertBanner(ctx, store.InsertBannerParams{
			Name: clip(name, 100), ContentType: ctype, SizeBytes: int32(len(data)), StorageKey: key, //nolint:gosec // at most 1 MB
		})
		if err != nil {
			return err
		}
		out = row
		return audit.Record(ctx, tx, audit.Entry{Action: "email_banner.added", EntityType: "email_banner", EntityID: row.ID, Changes: map[string]any{"name": row.Name}})
	})
	if err != nil {
		_ = s.files.Delete(key)
	}
	return out, err
}

// Banners lists the company's banners.
func (s *Service) Banners(ctx context.Context, p authz.Principal) ([]store.EmailBanner, error) {
	var out []store.EmailBanner
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).ListBanners(ctx)
		return err
	})
	return out, err
}

// DeleteBanner takes a banner out of the list; emails already queued with it keep it.
func (s *Service) DeleteBanner(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		n, err := store.New(tx).DeleteBanner(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("banner")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "email_banner.deleted", EntityType: "email_banner", EntityID: id, Changes: map[string]any{}})
	})
}

// BannerFile opens a banner's image.
func (s *Service) BannerFile(ctx context.Context, p authz.Principal, id string) (string, io.ReadCloser, error) {
	var row store.EmailBanner
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		found, err := store.New(tx).GetBanners(ctx, []string{id})
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return apperr.NotFound("banner")
		}
		row = found[0]
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	f, err := s.files.Open(row.StorageKey)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, apperr.NotFound("banner image")
	}
	return row.ContentType, f, err
}

// AddAttachment stores a file to attach to the next email the user sends.
// Uploads not sent within a day are removed.
func (s *Service) AddAttachment(ctx context.Context, p authz.Principal, fileName string, data []byte) (store.EmailAttachment, error) {
	if len(data) == 0 {
		var errs apperr.FieldErrors
		errs.Add("file", "is required and must not be empty")
		return store.EmailAttachment{}, errs.Err()
	}
	if len(data) > maxFileBytes {
		return store.EmailAttachment{}, apperr.New(api.ErrorCodePAYLOADTOOLARGE, "A file can be at most 10 MB.")
	}
	fileName = clip(strings.TrimSpace(filepath.Base(fileName)), 255)
	if fileName == "" || fileName == "." {
		fileName = "attachment"
	}
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	key := p.TenantID + "/eat_" + newKey()
	if err := s.files.Put(key, data); err != nil {
		return store.EmailAttachment{}, err
	}
	var out store.EmailAttachment
	var stale []string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if stale, err = q.DeleteStaleAttachments(ctx); err != nil {
			return err
		}
		out, err = q.InsertAttachment(ctx, store.InsertAttachmentParams{
			FileName: fileName, ContentType: clip(ctype, 255), SizeBytes: int32(len(data)), StorageKey: key, //nolint:gosec // at most 10 MB
		})
		return err
	})
	if err != nil {
		_ = s.files.Delete(key)
		return out, err
	}
	for _, k := range stale {
		if err := s.files.Delete(k); err != nil {
			logx.From(ctx).Warn("could not remove an old email upload", "error", err)
		}
	}
	return out, nil
}
