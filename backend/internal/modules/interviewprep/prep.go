package interviewprep

// Prep material: the AI writes it in the background from the job (its
// description and ad, never its salary), the client and the questions
// recorded at the job's interviews; the answer comes back on the queue.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviewprep/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// StartPrep queues the AI writing the job's prep material.
func (s *Service) StartPrep(ctx context.Context, p authz.Principal, jobID string, focus *string) (Prep, error) {
	focus = optional(focus)
	var out Prep
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		job, err := s.deps.Jobs.ForInterviewPrep(ctx, tx, p, jobID)
		if err != nil {
			return err
		}
		recorded, err := q.QuestionsForPrep(ctx, jobID)
		if err != nil {
			return err
		}
		if len(recorded) > MaxRecordedQuestions {
			recorded = recorded[:MaxRecordedQuestions] // the newest: what one answer can cover
		}
		text := JobText(job.Description, job.AdText)
		if !EnoughToPrepare(text, len(recorded)) {
			return apperr.New(api.ErrorCodeINTERVIEWPREPTOOLITTLE,
				"There is too little to prepare from: add the job's description or ad, or record questions asked at its interviews.")
		}
		var access *api.AiAccess
		on := false
		if s.deps.AI != nil {
			if access, on, err = s.deps.AI.Allowed(ctx, tx, p.TenantID); err != nil {
				return err
			}
		}
		if !on {
			return apperr.New(api.ErrorCodeINTERVIEWPREPAIOFF,
				"Prep material needs the AI: set up an AI provider in Settings, or wait for next month's allowance.")
		}
		company, err := s.deps.Companies.ForInterviewPrep(ctx, tx, job.CompanyID)
		if err != nil {
			return err
		}
		row, err := q.InsertInterviewPrep(ctx, store.InsertInterviewPrepParams{JobID: jobID, Focus: focus, QuestionCount: int32(len(recorded))}) //nolint:gosec // at most 60
		if err != nil {
			return err
		}
		questions := make([]api.InterviewPrepQuestion, len(recorded))
		for i, r := range recorded {
			// What people typed may hold pay ("said RM 12k"): never for the AI.
			questions[i] = api.InterviewPrepQuestion{Question: redact.WithoutMoney(r.Question), Kind: api.InterviewQuestionKind(r.Kind),
				Round: nullRound(r.Round), Notes: orNull(withoutMoney(r.Notes))}
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindInterviewPrepWrite, api.InterviewPrepWriteJob{
			PrepId: row.ID, JobTitle: job.Title, CompanyName: job.CompanyName, CompanyIndustry: orNull(company.Industry),
			CompanyAbout: orNull(withoutMoney(clipped(company.About, 2000))), Location: orNull(job.Location),
			WorkArrangement: orNull(job.WorkArrangement), JobDescription: text, Questions: questions, Focus: orNull(withoutMoney(focus)), Ai: access,
		}); err != nil {
			return err
		}
		out = prepFrom(row)
		return audit.Record(ctx, tx, audit.Entry{Action: "interview_prep.requested", EntityType: "job_order", EntityID: jobID,
			Changes: map[string]any{"prepId": row.ID, "questions": len(recorded)}})
	})
	return out, err
}

// Preps lists the job's prep material (the archived, or the others), newest first.
func (s *Service) Preps(ctx context.Context, p authz.Principal, jobID string, archived bool) ([]Prep, error) {
	var out []Prep
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.jobExists(ctx, tx, jobID); err != nil {
			return err
		}
		rows, err := store.New(tx).ListInterviewPreps(ctx, store.ListInterviewPrepsParams{JobID: jobID, Archived: archived})
		now := s.now()
		out = make([]Prep, len(rows))
		for i, r := range rows {
			out[i] = Shown(prepFrom(r), now)
		}
		return err
	})
	return out, err
}

// ArchivePrep archives prep material, or brings it back.
func (s *Service) ArchivePrep(ctx context.Context, p authz.Principal, id string, archived bool) (Prep, error) {
	var out Prep
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetInterviewPrep(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("interview prep")
		}
		if err != nil {
			return err
		}
		if err := s.jobExists(ctx, tx, row.JobID); err != nil {
			return apperr.NotFound("interview prep")
		}
		var at *time.Time
		if archived {
			now := s.now()
			if row.ArchivedAt != nil {
				now = *row.ArchivedAt
			}
			at = &now
		}
		saved, err := q.ArchiveInterviewPrep(ctx, store.ArchiveInterviewPrepParams{ID: id, ArchivedAt: at})
		if err != nil {
			return err
		}
		out = Shown(prepFrom(saved), s.now())
		action := map[bool]string{true: "interview_prep.archived", false: "interview_prep.restored"}[archived]
		return audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "job_order", EntityID: row.JobID, Changes: map[string]any{"prepId": id}})
	})
	return out, err
}

// written saves what the AI wrote (or why it could not), from the queue.
func (s *Service) written(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var in api.InterviewPrepWrittenJob
	if err := job.Decode(&in); err != nil {
		return err
	}
	params := store.FinishInterviewPrepParams{ID: in.PrepId, Status: "failed", Tokens: int32(in.InputTokens + in.OutputTokens)} //nolint:gosec // token counts are small
	if m, err := in.Model.Get(); err == nil {
		params.Model = &m
	}
	content, contentErr := in.Content.Get()
	switch {
	case contentErr == nil && (len(content.SoftSkills) > 0 || len(content.Technical) > 0):
		short := Short(content)
		raw, err := json.Marshal(short)
		if err != nil {
			return err
		}
		params.Status, params.Content = "done", raw
	default:
		reason := "The AI's answer could not be read. Try again."
		if e, err := in.Error.Get(); err == nil && strings.TrimSpace(e) != "" {
			reason = clip(e, 500)
		}
		params.Error = &reason
	}
	n, err := store.New(tx).FinishInterviewPrep(ctx, params)
	if err == nil && n == 0 {
		logx.From(ctx).Info("interview prep already finished or gone", "prep_id", in.PrepId)
	}
	return err
}

func (s *Service) tokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).InterviewPrepTokensSince(ctx, since)
}

func prepFrom(r store.InterviewPrep) Prep {
	p := Prep{ID: r.ID, JobID: r.JobID, Status: r.Status, Focus: r.Focus, Error: r.Error, Model: r.Model,
		QuestionCount: int(r.QuestionCount), Archived: r.ArchivedAt != nil, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt}
	if len(r.Content) > 0 {
		var c api.InterviewPrepContent
		if json.Unmarshal(r.Content, &c) == nil {
			p.Content = &c
		}
	}
	return p
}

func orNull(s *string) nullable.Nullable[string] {
	if s == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*s)
}

func nullRound(s *string) nullable.Nullable[api.InterviewRound] {
	if s == nil {
		return nullable.NewNullNullable[api.InterviewRound]()
	}
	return nullable.NewNullableWithValue(api.InterviewRound(*s))
}

func withoutMoney(s *string) *string {
	if s == nil {
		return nil
	}
	t := redact.WithoutMoney(*s)
	return &t
}

func clipped(s *string, size int) *string {
	if s == nil {
		return nil
	}
	c := clip(*s, size)
	return &c
}
