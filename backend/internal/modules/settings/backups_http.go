package settings

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
)

// GetBackups handles GET /v1/settings/backups.
func (h *HTTP) GetBackups(ctx context.Context, _ api.GetBackupsRequestObject) (api.GetBackupsResponseObject, error) {
	b, err := h.svc.Backups(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetBackups200JSONResponse(backupsToAPI(b)), nil
}

// SetAutoBackup handles PUT /v1/settings/backups/auto.
func (h *HTTP) SetAutoBackup(ctx context.Context, req api.SetAutoBackupRequestObject) (api.SetAutoBackupResponseObject, error) {
	b, err := h.svc.SetAutoBackup(ctx, authz.MustFromContext(ctx), req.Body.Auto)
	if err != nil {
		return nil, err
	}
	return api.SetAutoBackup200JSONResponse(backupsToAPI(b)), nil
}

// RequestBackup handles POST /v1/settings/backups/run.
func (h *HTTP) RequestBackup(ctx context.Context, _ api.RequestBackupRequestObject) (api.RequestBackupResponseObject, error) {
	b, err := h.svc.RequestBackup(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.RequestBackup202JSONResponse(backupsToAPI(b)), nil
}

// DeleteBackup handles DELETE /v1/settings/backups/{backupName}.
func (h *HTTP) DeleteBackup(ctx context.Context, req api.DeleteBackupRequestObject) (api.DeleteBackupResponseObject, error) {
	b, err := h.svc.DeleteBackup(ctx, authz.MustFromContext(ctx), req.BackupName)
	if err != nil {
		return nil, err
	}
	return api.DeleteBackup200JSONResponse(backupsToAPI(b)), nil
}

// CreateBackupDownloadLink handles POST /v1/settings/backups/{backupName}/link.
func (h *HTTP) CreateBackupDownloadLink(ctx context.Context, req api.CreateBackupDownloadLinkRequestObject) (api.CreateBackupDownloadLinkResponseObject, error) {
	l, err := h.svc.DownloadLink(ctx, authz.MustFromContext(ctx), req.BackupName)
	if err != nil {
		return nil, err
	}
	return api.CreateBackupDownloadLink200JSONResponse{Url: l.URL, FileName: l.FileName, SizeBytes: l.Size, ExpiresAt: l.ExpiresAt}, nil
}

// DownloadBackup handles GET /v1/backup-downloads/{backupName}: the backup as
// one .tar, sent as it is read (a backup can be many gigabytes).
func (h *HTTP) DownloadBackup(ctx context.Context, req api.DownloadBackupRequestObject) (api.DownloadBackupResponseObject, error) {
	size, err := h.svc.OpenDownload(ctx, req.BackupName, req.Params.Token)
	if err != nil {
		return nil, err
	}
	return tarDownload{dir: h.svc.backups.Dir, name: req.BackupName, size: size}, nil
}

// tarDownload streams a backup. It lifts the server's write time limit for
// this response only: a large backup takes longer than that to send.
type tarDownload struct {
	dir, name string
	size      int64
}

func (t tarDownload) VisitDownloadBackupResponse(w http.ResponseWriter) error {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Length", strconv.FormatInt(t.size, 10))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar"`, t.name))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	return backup.WriteTar(w, t.dir, t.name)
}

func backupsToAPI(b Backups) api.Backups {
	out := api.Backups{Available: b.Available, Auto: b.Control.Auto, ServiceRunning: b.Control.ServiceRunning, Backups: []api.BackupFile{}, TotalBytes: b.Total,
		ServiceSeenAt: c.OrNull(b.Control.ServiceSeenAt), RequestedAt: c.OrNull(b.Control.RequestedAt)}
	out.Last.SetNull()
	if l := b.Last; l != nil {
		out.Last.Set(api.BackupStatus{Name: l.Name, StartedAt: l.StartedAt, FinishedAt: c.OrNull(l.FinishedAt),
			Status: api.BackupStatusStatus(l.Status), SizeBytes: c.OrNull(l.SizeBytes), Error: c.OrNull(l.Error)})
	}
	for _, f := range b.List {
		out.Backups = append(out.Backups, api.BackupFile{Name: f.Name, CreatedAt: f.CreatedAt, SizeBytes: f.SizeBytes, Files: f.Files})
	}
	return out
}
