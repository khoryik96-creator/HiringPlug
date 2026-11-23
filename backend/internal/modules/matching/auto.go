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
	Why   *api.MeaningWhy // found by meaning: why
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
	p := authz.System(tenantID)
	q := store.New(tx)
	var slots int64
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
		if slots == 0 {
			slots, err = s.allowanceSlots(ctx, tx, tenantID)
			if err != nil {
				return skipIfExpected(err)
			}
		}
		if _, err := s.queue(ctx, tx, p, ai, a, job, cv); err != nil {
			return err
		}
		slots--
		if slots == 0 {
			break
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

// Suggest returns possible candidates for a job: those not on it whose work
// matches the job by meaning (its title and needs), then those whose CVs
// share its distinctive words, best first, leaving out those who withdrew
// consent and (by the database) those the user may not see. needsText is
// true, with no suggestions, for a job with no text to search with yet.
// meaning says how the job was read by meaning (nil: not asked).
func (s *Service) Suggest(ctx context.Context, p authz.Principal, jobID string) (out []Suggestion, needsText bool, meaning *api.MeaningOutcome, err error) {
	if !p.Can(api.PermissionCandidatesRead) || !p.Can(api.PermissionDocumentsRead) || !p.Can(api.PermissionPipelineRead) {
		return nil, false, nil, apperr.New(api.ErrorCodeFORBIDDEN, "Finding possible candidates reads CVs and the job's pipeline, which you may not see.")
	}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if !p.Can(api.PermissionJobsRead) {
			return apperr.NotFound("job")
		}
		j, err := s.deps.Jobs.ForMatching(ctx, tx, p, jobID)
		if err != nil {
			return err
		}
		if _, ok := JobText(j); !ok {
			needsText = true
			return nil
		}
		onJob, err := s.deps.Applications.CandidatesOnJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		hits, err := s.deps.CVs.SearchCVs(ctx, tx, SearchTerms(j), onJob, 2*maxSuggestions)
		if err != nil {
			return err
		}
		var found MeaningFound
		if s.deps.Meaning != nil {
			if found, err = s.deps.Meaning.ForJob(ctx, tx, p, j.Title, JobMeaningText(j), onJob, hits, 2*maxSuggestions); err != nil {
				return err
			}
			meaning = &found.Outcome
		}
		ids := suggestionOrder(found, hits)
		briefs, err := s.deps.Candidates.Briefs(ctx, tx, ids)
		if err != nil {
			return err
		}
		byWords := map[string]CVHit{}
		for _, h := range hits {
			byWords[h.CandidateID] = h
		}
		why := map[string]MeaningMatch{}
		for _, m := range found.Matches {
			why[m.CandidateID] = m
		}
		for _, id := range ids {
			b, ok := briefs[id]
			if !ok || len(out) == maxSuggestions {
				continue
			}
			if no, err := s.consentWithdrawn(ctx, tx, id); err != nil {
				return err
			} else if no {
				continue
			}
			hit, found := byWords[id]
			if !found {
				hit = CVHit{CandidateID: id}
			}
			sug := Suggestion{CVHit: hit, Name: b.Name, Title: b.Title}
			if m, ok := why[id]; ok {
				w := m.Why
				sug.Why, sug.Rank = &w, min(1, m.Score)
			}
			out = append(out, sug)
		}
		return nil
	})
	return out, needsText, meaning, err
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
