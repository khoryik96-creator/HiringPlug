package settings

// Settings > System after a restore: the jobs it held because they act
// outside HiringPlug (emails, calendar updates, webhooks, automations),
// each released or cancelled, or all at once. Audited like a retried job.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

const heldShown = 200

// settleHeld releases (or cancels) one held job of the company, or all of
// them (jobID ""), and returns the new status.
func (s *Service) settleHeld(ctx context.Context, p authz.Principal, jobID string, release bool) (SystemStatus, error) {
	var out SystemStatus
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		settle, action, all := jobqueue.CancelHeld, "settings.job_cancelled", "settings.held_jobs_cancelled"
		if release {
			settle, action, all = jobqueue.Release, "settings.job_released", "settings.held_jobs_released"
		}
		n, err := settle(ctx, tx, p.TenantID, jobID)
		if err != nil {
			return err
		}
		if jobID != "" && n == 0 {
			return apperr.NotFound("held job")
		}
		entry := audit.Entry{Action: action, EntityType: "job", EntityID: jobID}
		if jobID == "" {
			entry = audit.Entry{Action: all, EntityType: "tenant", EntityID: p.TenantID, Changes: map[string]any{"jobs": n}}
		}
		if n > 0 {
			if err := audit.Record(ctx, tx, entry); err != nil {
				return err
			}
		}
		out, err = systemStatus(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}

// ReleaseHeldJob handles POST /v1/settings/system/jobs/{jobId}/release.
func (h *HTTP) ReleaseHeldJob(ctx context.Context, req api.ReleaseHeldJobRequestObject) (api.ReleaseHeldJobResponseObject, error) {
	st, err := h.svc.settleHeld(ctx, authz.MustFromContext(ctx), req.JobId, true)
	if err != nil {
		return nil, err
	}
	return api.ReleaseHeldJob200JSONResponse(systemToAPI(st)), nil
}

// CancelHeldJob handles POST /v1/settings/system/jobs/{jobId}/cancel.
func (h *HTTP) CancelHeldJob(ctx context.Context, req api.CancelHeldJobRequestObject) (api.CancelHeldJobResponseObject, error) {
	st, err := h.svc.settleHeld(ctx, authz.MustFromContext(ctx), req.JobId, false)
	if err != nil {
		return nil, err
	}
	return api.CancelHeldJob200JSONResponse(systemToAPI(st)), nil
}

// SettleHeldJobs handles POST /v1/settings/system/held.
func (h *HTTP) SettleHeldJobs(ctx context.Context, req api.SettleHeldJobsRequestObject) (api.SettleHeldJobsResponseObject, error) {
	st, err := h.svc.settleHeld(ctx, authz.MustFromContext(ctx), "", req.Body.Action == api.SettleHeldJobsJSONBodyActionRelease)
	if err != nil {
		return nil, err
	}
	return api.SettleHeldJobs200JSONResponse(systemToAPI(st)), nil
}

func heldToAPI(st SystemStatus, out *api.SystemStatus) {
	out.Held = []api.HeldJob{}
	for _, j := range st.Held {
		out.Held = append(out.Held, api.HeldJob{Id: j.ID, Kind: j.Kind, Attempts: j.Attempts, CreatedAt: j.CreatedAt,
			HeldAt: c.OrNull(j.HeldAt), RunAfter: j.RunAfter})
	}
	out.Restore.SetNull()
	if r := st.Restore; r != nil {
		out.Restore.Set(api.RestoreInfo{BackupName: r.BackupName, RestoredAt: r.RestoredAt, FromAnotherServer: r.FromAnotherServer(),
			Held: r.Held, Resumed: r.Resumed, ReleasedAt: c.OrNull(r.ReleasedAt)})
	}
}
