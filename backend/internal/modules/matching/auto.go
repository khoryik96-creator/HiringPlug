package matching

// Scoring without being asked (while an AI provider is set up): a candidate
// put on a job is scored, and so are a candidate's open jobs when a new CV
// of theirs is read. Also possible candidates for a job, found by the words
// of their CVs (no AI needed).

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// maxSuggestions is how many possible candidates are shown for a job.
const maxSuggestions = 20

// Suggestion is a possible candidate for a job.
type Suggestion struct {
	CVHit
	Name  string
	Title *string
}

// WhenAdded scores a candidate just put on a job, in the caller's
// transaction (a hook of the pipeline module).
func (m *Module) WhenAdded(ctx context.Context, tx pgx.Tx, tenantID string, a Application) error {
	return m.service.autoScore(ctx, tx, tenantID, []Application{a})
}

// WhenCVRead scores a candidate's open jobs again once a new CV of theirs
// has been read, in the caller's transaction (a hook of the documents module).
func (m *Module) WhenCVRead(ctx context.Context, tx pgx.Tx, tenantID, candidateID string) error {
	apps, err := m.service.deps.Applications.OpenOfCandidate(ctx, tx, candidateID)
	if err != nil {
		return err
	}
	return m.service.autoScore(ctx, tx, tenantID, apps)
}

// autoScore queues a score for each application, as the app (not a person),
// leaving out those it cannot score now: matching off, the allowance used,
// already waiting, consent withdrawn, no CV text yet, no job text. Only a
// database failure is an error (it undoes what triggered it).
func (s *Service) autoScore(ctx context.Context, tx pgx.Tx, tenantID string, apps []Application) error {
	if len(apps) == 0 {
		return nil
	}
	ai, on, err := s.deps.AI.Access(ctx, tx)
	if err != nil || !on {
		return err
	}
	if err := s.withinAllowance(ctx, tx); err != nil {
		return skipIfExpected(err)
	}
	p := authz.System(tenantID)
	q := store.New(tx)
	for _, a := range apps {
		latest, err := q.LatestForApplication(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(latest) == 1 && latest[0].Status == "queued" {
			continue
		}
		if no, err := s.consentWithdrawn(ctx, tx, a.CandidateID); err != nil || no {
			if err != nil {
				return err
			}
			continue
		}
		job, err := s.jobText(ctx, tx, p, a.JobID)
		if err != nil {
			if err := skipIfExpected(err); err != nil {
				return err
			}
			continue
		}
		cv, err := s.cvText(ctx, tx, p, a.CandidateID)
		if err != nil {
			if err := skipIfExpected(err); err != nil {
				return err
			}
			continue
		}
		if cv == "" {
			continue // scored once a CV has been read
		}
		if _, err := s.queue(ctx, tx, p, ai, a, job, cv); err != nil {
			return err
		}
	}
	return nil
}

// skipIfExpected turns the reasons not to score (an apperr: limit, no text,
// not found) into nil, and keeps real failures.
func skipIfExpected(err error) error {
	var e *apperr.Error
	if errors.As(err, &e) {
		return nil
	}
	return err
}

// Suggest returns possible candidates for a job: those not on it whose CVs
// share the most words with the job's text, best first, leaving out those
// who withdrew consent and (by the database) those the user may not see.
func (s *Service) Suggest(ctx context.Context, p authz.Principal, jobID string) ([]Suggestion, error) {
	if !p.Can(api.PermissionCandidatesRead) || !p.Can(api.PermissionDocumentsRead) || !p.Can(api.PermissionPipelineRead) {
		return nil, apperr.New(api.ErrorCodeFORBIDDEN, "Finding possible candidates reads CVs and the job's pipeline, which you may not see.")
	}
	var out []Suggestion
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if !p.Can(api.PermissionJobsRead) {
			return apperr.NotFound("job")
		}
		j, err := s.deps.Jobs.ForMatching(ctx, tx, p, jobID)
		if err != nil {
			return err
		}
		if _, ok := JobText(j); !ok {
			return apperr.New(api.ErrorCodeMATCHNEEDSTEXT, "This job has no description or ad text to look for candidates with yet: add one first.")
		}
		onJob, err := s.deps.Applications.CandidatesOnJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		hits, err := s.deps.CVs.SearchCVs(ctx, tx, SearchText(j), onJob, 2*maxSuggestions)
		if err != nil {
			return err
		}
		ids := make([]string, len(hits))
		for i, h := range hits {
			ids[i] = h.CandidateID
		}
		briefs, err := s.deps.Candidates.Briefs(ctx, tx, ids)
		if err != nil {
			return err
		}
		for _, h := range hits {
			b, ok := briefs[h.CandidateID]
			if !ok || len(out) == maxSuggestions {
				continue
			}
			if no, err := s.consentWithdrawn(ctx, tx, h.CandidateID); err != nil {
				return err
			} else if no {
				continue
			}
			out = append(out, Suggestion{CVHit: h, Name: b.Name, Title: b.Title})
		}
		return nil
	})
	return out, err
}

func (s *Service) consentWithdrawn(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error) {
	if s.deps.Consent == nil {
		return false, nil
	}
	return s.deps.Consent.DoNotContact(ctx, tx, candidateID)
}

// currentHash identifies the job's text now ("" for a job that is gone), to
// tell which scores are outdated. It reveals nothing, so it reads as the app.
func (s *Service) currentHash(ctx context.Context, tx pgx.Tx, tenantID, jobID string) (string, error) {
	j, err := s.deps.Jobs.ForMatching(ctx, tx, authz.System(tenantID), jobID)
	if err != nil {
		return "", skipIfExpected(err)
	}
	text, _ := JobText(j)
	return Hash(text), nil
}
