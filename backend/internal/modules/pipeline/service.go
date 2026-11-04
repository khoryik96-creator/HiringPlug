package pipeline

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the pipeline use cases.
type Service struct {
	pool       *pgxpool.Pool
	candidates Candidates
	jobs       Jobs
	companies  Companies
	whenAdded  []AddedHook
}

// AddedHook runs when a candidate is put on a job, in the same transaction
// (e.g. CV vs job matching scores them). An error undoes the addition.
type AddedHook func(ctx context.Context, tx pgx.Tx, tenantID, jobID string, a OnJob) error

func (s *Service) added(ctx context.Context, tx pgx.Tx, tenantID string, a OnJob, jobID string) error {
	for _, hook := range s.whenAdded {
		if err := hook(ctx, tx, tenantID, jobID, a); err != nil {
			return err
		}
	}
	return nil
}

// Add puts a candidate on a job at an open stage (applied by default).
func (s *Service) Add(ctx context.Context, p authz.Principal, jobID, candidateID, stage string) (View, error) {
	if stage == "" {
		stage = Applied
	}
	if err := CheckNewStage(stage).Err(); err != nil {
		return View{}, err
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.requireJob(ctx, tx, jobID); err != nil {
			return err
		}
		ok, err := s.candidates.Exists(ctx, tx, candidateID)
		if err != nil {
			return err
		}
		if !ok {
			var errs apperr.FieldErrors
			errs.Add("candidateId", "no such candidate")
			return errs.Err()
		}
		q := store.New(tx)
		row, err := q.InsertApplication(ctx, store.InsertApplicationParams{JobOrderID: jobID, CandidateID: candidateID, Stage: stage})
		if database.IsNoRows(err) { // already on this job
			existing, findErr := q.FindApplication(ctx, store.FindApplicationParams{JobOrderID: jobID, CandidateID: candidateID})
			if findErr != nil {
				return findErr
			}
			return apperr.New(api.ErrorCodeAPPLICATIONEXISTS, "The candidate is already on this job.").WithDetail("existingApplicationId", existing)
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "application.created", EntityType: "application", EntityID: row.ID,
			Changes: map[string]any{"jobId": jobID, "candidateId": candidateID, "stage": stage},
		}); err != nil {
			return err
		}
		if err := s.added(ctx, tx, p.TenantID, OnJob{ID: row.ID, CandidateID: candidateID, Stage: stage}, jobID); err != nil {
			return err
		}
		views, err := s.views(ctx, tx, []store.Application{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

// Move changes an application's stage, following the rules in domain.go.
// expectedVersion must be the version the user loaded.
func (s *Service) Move(ctx context.Context, p authz.Principal, id string, expectedVersion int, m Move) (View, error) {
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetApplication(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("application")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("application").WithDetail("currentVersion", row.Version)
		}
		current := fromRow(row)
		move, changed, err := CheckMove(current, m)
		var notAllowed MoveNotAllowedError
		if errors.As(err, &notAllowed) {
			return apperr.New(api.ErrorCodeSTAGEMOVENOTALLOWED, notAllowed.Message)
		}
		if err != nil {
			return err
		}
		if changed {
			row, err = q.MoveApplication(ctx, store.MoveApplicationParams{
				ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small positive integers
				Stage: move.To, Reason: move.Reason, ReasonNote: move.Note,
			})
			if database.IsNoRows(err) {
				return apperr.StaleVersion("application")
			}
			if err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "application.moved", EntityType: "application", EntityID: id,
				Changes: audit.Diff(
					map[string]any{"stage": current.Stage, "reason": current.Reason, "reasonNote": current.ReasonNote},
					map[string]any{"stage": move.To, "reason": move.Reason, "reasonNote": move.Note},
				),
			}); err != nil {
				return err
			}
		}
		views, err := s.views(ctx, tx, []store.Application{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

// ForJob returns a job's pipeline, in the order candidates were added.
func (s *Service) ForJob(ctx context.Context, p authz.Principal, jobID string) ([]View, error) {
	var out []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.requireJob(ctx, tx, jobID); err != nil {
			return err
		}
		rows, err := store.New(tx).ListJobApplications(ctx, jobID)
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, rows)
		return err
	})
	return out, err
}

// ForCandidate returns the jobs a candidate is on, newest first.
func (s *Service) ForCandidate(ctx context.Context, p authz.Principal, candidateID string) ([]View, error) {
	var out []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ok, err := s.candidates.Exists(ctx, tx, candidateID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.NotFound("candidate")
		}
		rows, err := store.New(tx).ListCandidateApplications(ctx, candidateID)
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, rows)
		return err
	})
	return out, err
}

func (s *Service) requireJob(ctx context.Context, tx pgx.Tx, jobID string) error {
	ok, err := s.jobs.Exists(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NotFound("job")
	}
	return nil
}

// views adds job titles, company names and candidate names, with one lookup
// per module for the whole list.
func (s *Service) views(ctx context.Context, tx pgx.Tx, rows []store.Application) ([]View, error) {
	jobIDs, candidateIDs := make([]string, len(rows)), make([]string, len(rows))
	for i, r := range rows {
		jobIDs[i], candidateIDs[i] = r.JobOrderID, r.CandidateID
	}
	jobs, err := s.jobs.Info(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	companyIDs := make([]string, 0, len(jobs))
	for _, j := range jobs {
		companyIDs = append(companyIDs, j.CompanyID)
	}
	companies, err := s.companies.Names(ctx, tx, companyIDs)
	if err != nil {
		return nil, err
	}
	candidates, err := s.candidates.Info(ctx, tx, candidateIDs)
	if err != nil {
		return nil, err
	}
	out := make([]View, len(rows))
	for i, r := range rows {
		job := jobs[r.JobOrderID]
		candidate := candidates[r.CandidateID]
		out[i] = View{
			Application: fromRow(r), JobTitle: job.Title, CompanyID: job.CompanyID, CompanyName: companies[job.CompanyID],
			CandidateName: candidate.Name, CandidateTitle: candidate.Title,
		}
	}
	return out, nil
}

func fromRow(r store.Application) Application {
	return Application{
		ID: r.ID, Version: int(r.Version), JobID: r.JobOrderID, CandidateID: r.CandidateID, Stage: r.Stage,
		Reason: r.Reason, ReasonNote: r.ReasonNote, Channel: r.Channel, StageChangedAt: r.StageChangedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
