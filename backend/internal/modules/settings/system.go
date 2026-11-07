package settings

// Settings > System: the company's background jobs (waiting, retrying,
// failed for good, with "Try again") and the server's last backup.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

const failedShown = 50

// SystemStatus is the company's background jobs and the server's last backup.
type SystemStatus struct {
	Jobs   []jobqueue.KindStatus
	Failed []jobqueue.DeadJob
	Backup *backup.Last
}

// System returns the company's background jobs and the last backup.
func (s *Service) System(ctx context.Context, p authz.Principal) (SystemStatus, error) {
	var out SystemStatus
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = systemStatus(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}

func systemStatus(ctx context.Context, tx pgx.Tx, tenantID string) (SystemStatus, error) {
	var out SystemStatus
	var err error
	if out.Jobs, err = jobqueue.Summary(ctx, tx, tenantID); err != nil {
		return out, err
	}
	if out.Failed, err = jobqueue.Dead(ctx, tx, tenantID, failedShown); err != nil {
		return out, err
	}
	last, ok, err := backup.LastRun(ctx, tx)
	if ok {
		out.Backup = &last
	}
	return out, err
}

// RetryJob tries one of the company's failed jobs again.
func (s *Service) RetryJob(ctx context.Context, p authz.Principal, jobID string) (SystemStatus, error) {
	var out SystemStatus
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ok, err := jobqueue.Retry(ctx, tx, p.TenantID, jobID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.NotFound("failed job")
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "settings.job_retried", EntityType: "job", EntityID: jobID}); err != nil {
			return err
		}
		out, err = systemStatus(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}

// GetSystemStatus handles GET /v1/settings/system.
func (h *HTTP) GetSystemStatus(ctx context.Context, _ api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
	st, err := h.svc.System(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetSystemStatus200JSONResponse(systemToAPI(st)), nil
}

// RetryBackgroundJob handles POST /v1/settings/system/jobs/{jobId}/retry.
func (h *HTTP) RetryBackgroundJob(ctx context.Context, req api.RetryBackgroundJobRequestObject) (api.RetryBackgroundJobResponseObject, error) {
	st, err := h.svc.RetryJob(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	return api.RetryBackgroundJob200JSONResponse(systemToAPI(st)), nil
}

func systemToAPI(st SystemStatus) api.SystemStatus {
	out := api.SystemStatus{Jobs: []api.JobKindStatus{}, Failed: []api.FailedJob{}}
	for _, k := range st.Jobs {
		out.Jobs = append(out.Jobs, api.JobKindStatus{Kind: k.Kind, Waiting: k.Waiting, Running: k.Running, Retrying: k.Retrying,
			Failed: k.Dead, OldestWaitingSince: c.OrNull(k.OldestWaiting)})
	}
	for _, d := range st.Failed {
		out.Failed = append(out.Failed, api.FailedJob{Id: d.ID, Kind: d.Kind, Attempts: d.Attempts, Error: d.LastError, FailedAt: d.FinishedAt})
	}
	out.Backup.SetNull()
	if b := st.Backup; b != nil {
		out.Backup.Set(api.BackupStatus{Name: b.Name, StartedAt: b.StartedAt, FinishedAt: c.OrNull(b.FinishedAt),
			Status: api.BackupStatusStatus(b.Status), SizeBytes: c.OrNull(b.SizeBytes), Error: c.OrNull(b.Error)})
	}
	return out
}
