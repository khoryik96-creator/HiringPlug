package settings

// Settings > System after a restore: the jobs it held because they act
// outside HyPlug (emails, calendar updates, webhooks, automations) or
// ask a paid AI, each released or cancelled, or all at once. Audited like a
// retried job. A company's owners and admins see and settle their own; the
// agency that runs the server (runsServer) every company's, so one restore
// never waits on a company nobody looks after (hp jobs does the same on the
// server). Other companies learn only that some of their jobs were held
// after a restore of the server, and how many: never the backup's name nor
// other companies' counts.

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
		everyCompany, err := s.runsServer(ctx, tx, p)
		if err != nil {
			return err
		}
		scope := p.TenantID
		if everyCompany {
			scope = jobqueue.Everywhere
		}
		n, err := settle(ctx, tx, scope, jobID)
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
		if everyCompany {
			if entry.Changes == nil {
				entry.Changes = map[string]any{}
			}
			entry.Changes["everyCompany"] = true
		}
		if n > 0 {
			if err := audit.Record(ctx, tx, entry); err != nil {
				return err
			}
		}
		out, err = s.systemStatus(ctx, tx, p)
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

// heldStatus fills in the held jobs and the last restore as p may see them.
func (s *Service) heldStatus(ctx context.Context, tx pgx.Tx, p authz.Principal, out *SystemStatus) error {
	everyCompany, err := s.runsServer(ctx, tx, p)
	if err != nil {
		return err
	}
	scope := p.TenantID
	if everyCompany {
		scope = jobqueue.Everywhere
	}
	if out.Held, err = jobqueue.Held(ctx, tx, scope, heldShown); err != nil {
		return err
	}
	out.HeldEveryCompany = everyCompany
	restored, ok, err := jobqueue.LastRestore(ctx, tx)
	switch {
	case err != nil || !ok:
		return err
	case everyCompany:
		out.Restore = &restored
	case len(out.Held) > 0: // only that, and how many of theirs
		out.Restore = &jobqueue.Restored{RestoredAt: restored.RestoredAt, Held: len(out.Held)}
	}
	return nil
}

func heldToAPI(st SystemStatus, out *api.SystemStatus) {
	out.Held = []api.HeldJob{}
	for _, j := range st.Held {
		h := api.HeldJob{Id: j.ID, Kind: j.Kind, Attempts: j.Attempts, CreatedAt: j.CreatedAt,
			HeldAt: c.OrNull(j.HeldAt), RunAfter: j.RunAfter}
		h.Company.SetNull()
		if st.HeldEveryCompany {
			h.Company.Set(j.Company)
		}
		out.Held = append(out.Held, h)
	}
	out.HeldEveryCompany = st.HeldEveryCompany
	out.Restore.SetNull()
	if r := st.Restore; r != nil {
		info := api.RestoreInfo{EveryCompany: st.HeldEveryCompany, RestoredAt: r.RestoredAt, Held: r.Held}
		info.BackupName.SetNull()
		info.FromAnotherServer.SetNull()
		info.ReleasedAt.SetNull()
		if st.HeldEveryCompany {
			info.BackupName.Set(r.BackupName)
			info.FromAnotherServer.Set(r.FromAnotherServer())
			info.Resumed = r.Resumed
			info.ReleasedAt = c.OrNull(r.ReleasedAt)
		}
		out.Restore.Set(info)
	}
}
