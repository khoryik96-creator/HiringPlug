package assistant

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// Service holds the assistant's use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
}

// Status is whether the assistant is on and how much of this month's AI
// limit the company used (every AI feature together).
type Status struct {
	Enabled bool
	MonthUse
}

// Status returns whether the assistant is on, and this month's use.
func (s *Service) Status(ctx context.Context, p authz.Principal) (Status, error) {
	var out Status
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		if _, out.Enabled, err = s.deps.AI.Access(ctx, tx); err != nil {
			return err
		}
		out.MonthUse, err = s.deps.Usage.ThisMonth(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}

// List returns the user's own latest answers about one subject.
func (s *Service) List(ctx context.Context, p authz.Principal, subjectType, subjectID string) ([]store.AssistantAnswer, error) {
	var out []store.AssistantAnswer
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).ListAnswers(ctx, store.ListAnswersParams{SubjectType: subjectType, SubjectID: subjectID})
		if err != nil || len(out) == 0 {
			return err
		}
		switch subjectType {
		case "candidate":
			_, err = s.deps.Candidates.ForAssistant(ctx, tx, p, subjectID)
		case "application":
			_, err = s.deps.Applications.ForAssistant(ctx, tx, subjectID)
		case "job":
			_, err = s.deps.Jobs.ForAssistant(ctx, tx, p, subjectID)
		}
		var hidden *apperr.Error
		if errors.As(err, &hidden) && hidden.Code == api.ErrorCodeNOTFOUND {
			out = nil
			return nil
		}
		return err
	})
	return out, err
}

// Ask queues a question for the AI, with what the user may see of its
// subject.
func (s *Service) Ask(ctx context.Context, p authz.Principal, q Question) (store.AssistantAnswer, error) {
	q, err := CheckQuestion(q)
	if err != nil {
		return store.AssistantAnswer{}, err
	}
	var out store.AssistantAnswer
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		qs := store.New(tx)
		ai, err := s.ready(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		sections, person, err := s.sections(ctx, tx, p, q)
		if err != nil {
			return err
		}
		out, err = qs.InsertAnswer(ctx, store.InsertAnswerParams{
			SubjectType: q.SubjectType, SubjectID: q.SubjectID, Kind: q.Kind, Question: q.Question, UsedCv: slices.ContainsFunc(sections, func(s Section) bool { return s.Title == cvSectionTitle }),
		})
		if err != nil {
			return err
		}
		job := api.AssistantAskJob{
			AnswerId: out.ID, Kind: api.AssistantKind(q.Kind), SubjectType: ptr(api.AssistantSubjectType(q.SubjectType)), Question: apiconv.OrNull(CleanQuestion(q.Question, person)), Sections: toAPISections(sections), Ai: ai,
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindAssistantAsk, job); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "assistant.asked", EntityType: "assistant_answer", EntityID: out.ID,
			Changes: map[string]any{"subjectType": q.SubjectType, "subjectId": q.SubjectID, "kind": q.Kind},
		})
	})
	return out, err
}

// sections gathers what the AI may see of the subject: only what the user
// may see (a record they may not see is "not found"), and who the candidate
// is (to take them out of the question too).
func (s *Service) sections(ctx context.Context, tx pgx.Tx, p authz.Principal, q Question) ([]Section, redact.Identity, error) {
	var person redact.Identity
	candidate := func(id string) (Candidate, string, error) {
		if !p.Can(api.PermissionCandidatesRead) {
			return Candidate{}, "", apperr.NotFound("candidate")
		}
		c, err := s.deps.Candidates.ForAssistant(ctx, tx, p, id)
		person = redact.IdentityOf(c.Name, c.Email, c.Phone)
		if err != nil || !p.Can(api.PermissionDocumentsRead) {
			return c, "", err
		}
		text, err := s.deps.CVs.LatestCVText(ctx, tx, id)
		return c, text, err
	}
	job := func(id string) (Job, error) {
		if !p.Can(api.PermissionJobsRead) {
			return Job{}, apperr.NotFound("job")
		}
		return s.deps.Jobs.ForAssistant(ctx, tx, p, id)
	}
	switch q.SubjectType {
	case "candidate":
		c, text, err := candidate(q.SubjectID)
		return CandidateSections(c, text), person, err
	case "job":
		j, err := job(q.SubjectID)
		if err != nil || q.Kind != "market_map" || s.deps.Market == nil {
			return JobSections(j), person, err
		}
		pool, err := s.deps.Market.PoolText(ctx, tx, p, q.SubjectID)
		if err != nil || pool == "" {
			return JobSections(j), person, err
		}
		return append(JobSections(j), Section{Title: "In our own database", Text: pool}), person, nil
	default: // application
		if !p.Can(api.PermissionPipelineRead) {
			return nil, person, apperr.NotFound("application")
		}
		a, err := s.deps.Applications.ForAssistant(ctx, tx, q.SubjectID)
		if err != nil {
			return nil, person, err
		}
		c, text, err := candidate(a.CandidateID)
		if err != nil {
			return nil, person, err
		}
		j, err := job(a.JobID)
		return ApplicationSections(a, c, text, j), person, err
	}
}

// answered handles assistant.answered: the AI worker's answer, or why there
// is none, and the tokens it took.
func (s *Service) answered(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.AssistantAnsweredJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	params := store.SaveAnswerParams{
		ID: r.AnswerId, Status: "done", InputTokens: tokens(r.InputTokens), OutputTokens: tokens(r.OutputTokens),
	}
	if v, err := r.Answer.Get(); err == nil && v != "" {
		params.Answer = clipPtr(v, 20000)
	}
	if v, err := r.Model.Get(); err == nil && v != "" {
		params.Model = clipPtr(v, 200)
	}
	if v, err := r.Error.Get(); err == nil && v != "" {
		params.Status, params.Error = "failed", clipPtr(v, 1000)
	} else if params.Answer == nil {
		params.Status, params.Error = "failed", clipPtr("The AI gave no answer.", 1000)
	}
	asker, err := store.New(tx).SaveAnswer(ctx, params)
	if database.IsNoRows(err) {
		// Already answered (counted then: this adds nothing), or forgotten
		// meanwhile (what the AI used is still counted).
		return s.deps.Usage.RecordAI(ctx, tx, job.TenantID, "assistant", r.AnswerId, "", nil,
			int64(params.InputTokens), int64(params.OutputTokens))
	}
	if err != nil {
		return err
	}
	if err := s.deps.Usage.RecordAI(ctx, tx, job.TenantID, "assistant", r.AnswerId, "", &asker,
		int64(params.InputTokens), int64(params.OutputTokens)); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "assistant.answered", EntityType: "assistant_answer", EntityID: r.AnswerId,
		Changes: map[string]any{"status": params.Status, "tokens": params.InputTokens + params.OutputTokens},
	})
}

// tokens is a token count as saved: never below 0, at most 1<<30.
func tokens(n int) int32 {
	return int32(min(max(n, 0), 1<<30)) //nolint:gosec // within 0..1<<30
}

func clipPtr(s string, n int) *string {
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}
	return &s
}

func toAPISections(in []Section) []api.AssistantSection {
	out := make([]api.AssistantSection, len(in))
	for i, s := range in {
		out[i] = api.AssistantSection{Title: s.Title, Text: s.Text}
	}
	return out
}

func ptr[T any](v T) *T { return &v }
