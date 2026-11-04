package matching

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the matching use cases.
type Service struct {
	pool     *pgxpool.Pool
	deps     Deps
	settings Settings
	now      func() time.Time
}

// Match is a fit score of one application (or a request for one).
type Match struct {
	ID, ApplicationID, JobID, Status string
	Score                            *int
	Summary                          *string
	Reasons                          []Reason
	Dropped                          int
	Outdated                         bool
	JobHash                          *string
	Error, Model                     *string
	CreatedAt                        time.Time
	ScoredAt                         *time.Time
}

// JobResult is what scoring a job's candidates did.
type JobResult struct{ Queued, AlreadyScored, WithoutCV, ConsentWithdrawn int }

// maxBatch is how many candidates one "score them all" sends to the AI.
const maxBatch = 100

// Enabled reports whether matching is switched on.
func (s *Service) Enabled() bool { return s.settings.Enabled }

// List returns the latest score of each of a job's applications, or of one
// application (give exactly one of the IDs).
func (s *Service) List(ctx context.Context, p authz.Principal, jobID, applicationID string) ([]Match, error) {
	if (jobID == "") == (applicationID == "") {
		var errs apperr.FieldErrors
		errs.Add("jobId", "give jobId or applicationId (one of them)")
		return nil, errs.Err()
	}
	var out []Match
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var rows []store.LatestForJobRow
		var err error
		if jobID != "" {
			rows, err = q.LatestForJob(ctx, jobID)
		} else {
			var one []store.LatestForApplicationRow
			one, err = q.LatestForApplication(ctx, applicationID)
			for _, r := range one {
				rows = append(rows, store.LatestForJobRow(r))
			}
		}
		if err != nil {
			return err
		}
		hashes := map[string]string{} // the job's text now, per job
		for _, r := range rows {
			m, err := fromRow(r)
			if err != nil {
				return err
			}
			if _, ok := hashes[m.JobID]; !ok {
				if hashes[m.JobID], err = s.currentHash(ctx, tx, p.TenantID, m.JobID); err != nil {
					return err
				}
			}
			m.Outdated = Outdated(m.Status, m.JobHash, hashes[m.JobID])
			m.Reasons = Visible(m.Reasons, p.Can(api.PermissionDocumentsRead))
			out = append(out, m)
		}
		return nil
	})
	return out, err
}

// Score asks the AI how well an application's candidate fits its job. A
// score already waiting for the AI is returned instead of asking again.
func (s *Service) Score(ctx context.Context, p authz.Principal, applicationID string) (Match, error) {
	if err := s.on(); err != nil {
		return Match{}, err
	}
	var out Match
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		a, err := s.application(ctx, tx, p, applicationID)
		if err != nil {
			return err
		}
		latest, err := store.New(tx).LatestForApplication(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(latest) == 1 && latest[0].Status == "queued" {
			out, err = fromRow(store.LatestForJobRow(latest[0]))
			return err
		}
		if no, err := s.consentWithdrawn(ctx, tx, a.CandidateID); err != nil || no {
			if err != nil {
				return err
			}
			return apperr.New(api.ErrorCodeMATCHCONSENTWITHDRAWN,
				"This candidate withdrew consent to their data being used, so it is not sent to the AI.")
		}
		if err := s.withinAllowance(ctx, tx); err != nil {
			return err
		}
		job, err := s.jobText(ctx, tx, p, a.JobID)
		if err != nil {
			return err
		}
		cv, err := s.cvText(ctx, tx, p, a.CandidateID)
		if err != nil {
			return err
		}
		if cv == "" {
			return apperr.New(api.ErrorCodeMATCHNEEDSTEXT,
				"This candidate has no CV with readable text yet: upload one, or wait until it has been read.")
		}
		out, err = s.queue(ctx, tx, p, a, job, cv)
		return err
	})
	return out, err
}

// ScoreJob asks the AI about each of a job's active candidates not scored
// yet (all of them with rescore), at most maxBatch at a time. Candidates
// without a CV with readable text are counted and left out.
func (s *Service) ScoreJob(ctx context.Context, p authz.Principal, jobID string, rescore bool) (JobResult, error) {
	if err := s.on(); err != nil {
		return JobResult{}, err
	}
	var res JobResult
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		job, err := s.jobText(ctx, tx, p, jobID)
		if err != nil {
			return err
		}
		if !p.Can(api.PermissionPipelineRead) {
			return apperr.NotFound("job")
		}
		if err := s.withinAllowance(ctx, tx); err != nil {
			return err
		}
		apps, err := s.deps.Applications.OpenOnJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		latest, err := store.New(tx).LatestForJob(ctx, jobID)
		if err != nil {
			return err
		}
		status := make(map[string]string, len(latest))
		for _, m := range latest {
			status[m.ApplicationID] = m.Status
		}
		for _, a := range apps {
			if st := status[a.ID]; st == "queued" || (st == "done" && !rescore) {
				res.AlreadyScored++
				continue
			}
			if res.Queued == maxBatch {
				break
			}
			if no, err := s.consentWithdrawn(ctx, tx, a.CandidateID); err != nil {
				return err
			} else if no {
				res.ConsentWithdrawn++
				continue
			}
			cv, err := s.cvText(ctx, tx, p, a.CandidateID)
			if err != nil {
				return err
			}
			if cv == "" {
				res.WithoutCV++
				continue
			}
			if _, err := s.queue(ctx, tx, p, a, job, cv); err != nil {
				return err
			}
			res.Queued++
		}
		return nil
	})
	return res, err
}

func (s *Service) on() error {
	if !s.settings.Enabled {
		return apperr.New(api.ErrorCodeMATCHINGOFF, "Matching is switched off: no AI provider is set up yet.")
	}
	return nil
}

// withinAllowance refuses once the company used this month's AI allowance
// (matching and the assistant together).
func (s *Service) withinAllowance(ctx context.Context, tx pgx.Tx) error {
	since := monthStart(s.now(), s.settings.TimeZone)
	used, err := store.New(tx).TokensSince(ctx, since)
	if err != nil {
		return err
	}
	if s.deps.OtherAIUse != nil {
		other, err := s.deps.OtherAIUse.TokensSince(ctx, tx, since)
		if err != nil {
			return err
		}
		used += other
	}
	if used >= s.settings.MonthlyTokens {
		return apperr.New(api.ErrorCodeMATCHINGLIMITREACHED,
			"Your company has used this month's AI allowance; it starts again on the 1st.")
	}
	return nil
}

// application, jobText and cvText gather what the AI reads, as the user may
// see it: a record they may not see is "not found".
func (s *Service) application(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Application, error) {
	if !p.Can(api.PermissionPipelineRead) {
		return Application{}, apperr.NotFound("application")
	}
	return s.deps.Applications.ForMatching(ctx, tx, id)
}

func (s *Service) jobText(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (string, error) {
	if !p.Can(api.PermissionJobsRead) {
		return "", apperr.NotFound("job")
	}
	j, err := s.deps.Jobs.ForMatching(ctx, tx, p, id)
	if err != nil {
		return "", err
	}
	text, ok := JobText(j)
	if !ok {
		return "", apperr.New(api.ErrorCodeMATCHNEEDSTEXT,
			"This job has no description or ad text to compare CVs with yet: add one first.")
	}
	return text, nil
}

func (s *Service) cvText(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID string) (string, error) {
	if !p.Can(api.PermissionCandidatesRead) || !p.Can(api.PermissionDocumentsRead) {
		return "", apperr.New(api.ErrorCodeFORBIDDEN, "Scoring a candidate's fit reads their CV, which you may not see.")
	}
	c, err := s.deps.Candidates.ForMatching(ctx, tx, p, candidateID)
	if err != nil {
		return "", err
	}
	text, err := s.deps.CVs.LatestCVText(ctx, tx, candidateID)
	return CVText(c, text), err
}

// queue saves a match waiting for the AI, with what it is sent, and queues
// the question for the AI worker.
func (s *Service) queue(ctx context.Context, tx pgx.Tx, p authz.Principal, a Application, job, cv string) (Match, error) {
	hash := Hash(job)
	row, err := store.New(tx).InsertMatch(ctx, store.InsertMatchParams{ApplicationID: a.ID, JobID: a.JobID, SentJob: &job, SentCv: &cv, JobHash: &hash})
	if err != nil {
		return Match{}, err
	}
	if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindMatchScore, api.MatchScoreJob{MatchId: row.ID, Job: job, Cv: cv}); err != nil {
		return Match{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: "match.requested", EntityType: "match", EntityID: row.ID,
		Changes: map[string]any{"applicationId": a.ID, "jobId": a.JobID},
	}); err != nil {
		return Match{}, err
	}
	return fromRow(store.LatestForJobRow(row))
}

func fromRow(r store.LatestForJobRow) (Match, error) {
	m := Match{
		ID: r.ID, ApplicationID: r.ApplicationID, JobID: r.JobID, Status: r.Status, Summary: r.Summary,
		Dropped: int(r.Dropped), JobHash: r.JobHash, Error: r.Error, Model: r.Model, CreatedAt: r.CreatedAt, ScoredAt: r.ScoredAt,
	}
	if r.Score != nil {
		score := int(*r.Score)
		m.Score = &score
	}
	if err := json.Unmarshal(r.Reasons, &m.Reasons); err != nil {
		return Match{}, fmt.Errorf("match %s: reasons: %w", r.ID, err)
	}
	return m, nil
}

// monthStart is when the month that holds now began, in loc.
func monthStart(now time.Time, loc *time.Location) time.Time {
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
}
