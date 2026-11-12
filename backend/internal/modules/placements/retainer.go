package placements

// A job's retainer: read and set (one transaction each), and found for the
// placement it is taken off (the first on the job) and its invoice.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// RetainerView is a job's retainer and the placement it is taken off, if any.
type RetainerView struct {
	Retainer
	PlacementID *string
	Start       *time.Time // that placement's start (stages due after it)
}

// GetRetainer returns a job's retainer (version 0: none).
func (s *Service) GetRetainer(ctx context.Context, p authz.Principal, jobID string) (RetainerView, error) {
	var out RetainerView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.retainerOfJob(ctx, tx, jobID)
		return err
	})
	return out, err
}

// PutRetainer replaces a job's retainer, if nobody changed it since
// in.Version (0: there was none). No stages: the job has no retainer.
func (s *Service) PutRetainer(ctx context.Context, p authz.Principal, jobID string, in Retainer) (RetainerView, error) {
	if err := mayChangeMoney(p); err != nil {
		return RetainerView{}, err
	}
	in, errs := NormalizeRetainer(in, s.today())
	if err := errs.Err(); err != nil {
		return RetainerView{}, err
	}
	var out RetainerView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		before, err := s.retainerOfJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if before.Version != in.Version {
			return apperr.StaleVersion("retainer").WithDetail("currentVersion", before.Version)
		}
		q := store.New(tx)
		version := int32(in.Version) //nolint:gosec // versions are small
		switch {
		case len(in.Stages) == 0 && before.Version == 0:
			out = before
			return nil // nothing to clear
		case len(in.Stages) == 0:
			n, err := q.DeleteRetainer(ctx, store.DeleteRetainerParams{JobOrderID: jobID, ExpectedVersion: version})
			if err == nil && n == 0 {
				return apperr.StaleVersion("retainer")
			}
			if err != nil {
				return err
			}
		case before.Version == 0:
			_, err = q.InsertRetainer(ctx, store.InsertRetainerParams{JobOrderID: jobID, Currency: in.Currency, SignedOn: in.SignedOn})
		default:
			_, err = q.UpdateRetainer(ctx, store.UpdateRetainerParams{JobOrderID: jobID, ExpectedVersion: version, Currency: in.Currency, SignedOn: in.SignedOn})
		}
		if database.IsNoRows(err) {
			return apperr.StaleVersion("retainer")
		}
		if err != nil {
			return err
		}
		if len(in.Stages) > 0 {
			if err := q.DeleteRetainerStages(ctx, jobID); err != nil {
				return err
			}
			for i, st := range in.Stages {
				if err := q.InsertRetainerStage(ctx, store.InsertRetainerStageParams{
					JobOrderID: jobID, Position: int32(i), Label: st.Label, Amount: st.Amount, DueKind: st.DueKind, //nolint:gosec // at most 12
					DueOn: st.DueOn, DueDays: days32(st.DueDays), InvoiceStatus: st.InvoiceStatus,
					InvoiceNumber: st.InvoiceNumber, InvoicedOn: st.InvoicedOn, PaidOn: st.PaidOn,
				}); err != nil {
					return err
				}
			}
		}
		var signed any
		if in.SignedOn != nil {
			signed = in.SignedOn.Format(time.DateOnly)
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "job.retainer_set", EntityType: "job", EntityID: jobID, Changes: map[string]any{
			"stages": len(in.Stages), "signedOn": signed, "money": audit.Redacted, // never the amounts
		}}); err != nil {
			return err
		}
		out, err = s.retainerOfJob(ctx, tx, jobID)
		return err
	})
	return out, err
}

// retainerOfJob reads a job's retainer and the placement it is taken off.
func (s *Service) retainerOfJob(ctx context.Context, tx pgx.Tx, jobID string) (RetainerView, error) {
	ok, err := s.applications.JobExists(ctx, tx, jobID)
	if err != nil {
		return RetainerView{}, err
	}
	if !ok {
		return RetainerView{}, apperr.NotFound("job")
	}
	rs, err := loadRetainers(ctx, store.New(tx), []string{jobID})
	if err != nil {
		return RetainerView{}, err
	}
	r, ok := rs[jobID]
	if !ok {
		return RetainerView{}, nil
	}
	return s.withFirstPlacement(ctx, tx, jobID, r)
}

// withFirstPlacement adds the placement a retainer is taken off: the first
// on its job that was not cancelled.
func (s *Service) withFirstPlacement(ctx context.Context, tx pgx.Tx, jobID string, r Retainer) (RetainerView, error) {
	out := RetainerView{Retainer: r}
	appIDs, err := s.applications.PlacedOnJob(ctx, tx, jobID)
	if err != nil || len(appIDs) == 0 {
		return out, err
	}
	first, err := store.New(tx).FirstPlacementOf(ctx, appIDs)
	if database.IsNoRows(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.PlacementID, out.Start = &first.ID, &first.StartDate
	return out, nil
}

// retainersOff returns, by placement, the retainer of each job whose first
// placement it is, among some placements' jobs (nothing for users who may not
// see fees: the database hides retainers from them).
func (s *Service) retainersOff(ctx context.Context, tx pgx.Tx, jobIDs []string) (map[string]RetainerView, error) {
	rs, err := loadRetainers(ctx, store.New(tx), jobIDs)
	if err != nil || len(rs) == 0 {
		return nil, err
	}
	out := make(map[string]RetainerView, len(rs))
	for jobID, r := range rs {
		v, err := s.withFirstPlacement(ctx, tx, jobID, r)
		if err != nil {
			return nil, err
		}
		if v.PlacementID != nil {
			out[*v.PlacementID] = v
		}
	}
	return out, nil
}

func loadRetainers(ctx context.Context, q *store.Queries, jobIDs []string) (map[string]Retainer, error) {
	if len(jobIDs) == 0 {
		return map[string]Retainer{}, nil
	}
	heads, err := q.RetainersOfJobs(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Retainer, len(heads))
	for _, h := range heads {
		out[h.JobOrderID] = Retainer{Version: int(h.Version), Currency: h.Currency, SignedOn: h.SignedOn}
	}
	stages, err := q.RetainerStages(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	for _, st := range stages {
		r, ok := out[st.JobOrderID]
		if !ok {
			continue
		}
		var dueDays *int
		if st.DueDays != nil {
			d := int(*st.DueDays)
			dueDays = &d
		}
		r.Stages = append(r.Stages, RetainerStage{
			Label: st.Label, Amount: st.Amount, DueKind: st.DueKind, DueOn: st.DueOn, DueDays: dueDays,
			InvoiceStatus: st.InvoiceStatus, InvoiceNumber: st.InvoiceNumber, InvoicedOn: st.InvoicedOn, PaidOn: st.PaidOn,
		})
		out[st.JobOrderID] = r
	}
	return out, nil
}

func days32(d *int) *int32 {
	if d == nil {
		return nil
	}
	v := int32(*d) //nolint:gosec // 0..730
	return &v
}
