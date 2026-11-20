package interviewprep

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviewprep/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the interview_prep use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
	now  func() time.Time
	zone *time.Location
}

// QuestionChange is an edit of a question: nil fields are left as they are
// (Clear* empties an optional one), Archived archives it or brings it back.
type QuestionChange struct {
	Question, Kind           *string
	Round, Source, Notes     *string
	AskedOn                  *time.Time
	ClearRound, ClearSource  bool
	ClearNotes, ClearAskedOn bool
	Archived                 *bool
}

// today is the company's date, at midnight UTC (as dates are kept).
func (s *Service) today() time.Time {
	y, m, d := s.now().In(s.zone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// AddQuestion records a question asked at the job's interviews.
func (s *Service) AddQuestion(ctx context.Context, p authz.Principal, jobID string, f QuestionFields) (Question, error) {
	f, errs := Clean(f, s.today())
	if err := errs.Err(); err != nil {
		return Question{}, err
	}
	var out Question
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.jobExists(ctx, tx, jobID); err != nil {
			return err
		}
		row, err := store.New(tx).InsertInterviewQuestion(ctx, store.InsertInterviewQuestionParams{
			JobID: jobID, Question: f.Question, Round: f.Round, Kind: f.Kind, AskedOn: date(f.AskedOn), Source: f.Source, Notes: f.Notes,
		})
		if err != nil {
			return err
		}
		out = questionFrom(row)
		return audit.Record(ctx, tx, audit.Entry{Action: "interview_question.created", EntityType: "job_order", EntityID: jobID,
			Changes: map[string]any{"questionId": row.ID, "kind": f.Kind}})
	})
	return out, err
}

// Questions lists the job's questions (the archived ones, or the others), newest first.
func (s *Service) Questions(ctx context.Context, p authz.Principal, jobID string, archived bool) ([]Question, error) {
	var out []Question
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.jobExists(ctx, tx, jobID); err != nil {
			return err
		}
		rows, err := store.New(tx).ListInterviewQuestions(ctx, store.ListInterviewQuestionsParams{JobID: jobID, Archived: archived})
		out = make([]Question, len(rows))
		for i, r := range rows {
			out[i] = questionFrom(r)
		}
		return err
	})
	return out, err
}

// ChangeQuestion edits a question, or archives it (or brings it back), at the version the person saw.
func (s *Service) ChangeQuestion(ctx context.Context, p authz.Principal, id string, version int, c QuestionChange) (Question, error) {
	var out Question
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := s.ownQuestion(ctx, tx, id)
		if err != nil {
			return err
		}
		f := QuestionFields{Question: row.Question, Kind: row.Kind, Round: row.Round, Source: row.Source, Notes: row.Notes, AskedOn: dateOf(row.AskedOn)}
		apply(&f, c)
		f, errs := Clean(f, s.today())
		if err := errs.Err(); err != nil {
			return err
		}
		archivedAt := row.ArchivedAt
		if c.Archived != nil {
			archivedAt = nil
			if *c.Archived {
				now := s.now()
				if row.ArchivedAt != nil {
					now = *row.ArchivedAt
				}
				archivedAt = &now
			}
		}
		saved, err := q.UpdateInterviewQuestion(ctx, store.UpdateInterviewQuestionParams{
			ID: id, Version: int32(version), Question: f.Question, Round: f.Round, Kind: f.Kind, //nolint:gosec // versions are small
			AskedOn: date(f.AskedOn), Source: f.Source, Notes: f.Notes, ArchivedAt: archivedAt,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("interview question")
		}
		if err != nil {
			return err
		}
		out = questionFrom(saved)
		action := "interview_question.updated"
		if c.Archived != nil && (row.ArchivedAt != nil) != *c.Archived {
			action = map[bool]string{true: "interview_question.archived", false: "interview_question.restored"}[*c.Archived]
		}
		return audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "job_order", EntityID: row.JobID,
			Changes: map[string]any{"questionId": id}})
	})
	return out, err
}

// DeleteQuestion deletes a question recorded by mistake.
func (s *Service) DeleteQuestion(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := s.ownQuestion(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := store.New(tx).DeleteInterviewQuestion(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "interview_question.deleted", EntityType: "job_order", EntityID: row.JobID,
			Changes: map[string]any{"questionId": id}})
	})
}

// ownQuestion is a question of a job the user may see.
func (s *Service) ownQuestion(ctx context.Context, tx pgx.Tx, id string) (store.InterviewQuestion, error) {
	row, err := store.New(tx).GetInterviewQuestion(ctx, id)
	if database.IsNoRows(err) {
		return row, apperr.NotFound("interview question")
	}
	if err != nil {
		return row, err
	}
	if err := s.jobExists(ctx, tx, row.JobID); err != nil {
		return row, apperr.NotFound("interview question")
	}
	return row, nil
}

// jobExists is a NotFound error when the job is not the company's (or gone).
func (s *Service) jobExists(ctx context.Context, tx pgx.Tx, jobID string) error {
	ok, err := s.deps.Jobs.Exists(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NotFound("job")
	}
	return nil
}

func apply(f *QuestionFields, c QuestionChange) {
	if c.Question != nil {
		f.Question = *c.Question
	}
	if c.Kind != nil {
		f.Kind = *c.Kind
	}
	set := func(field **string, v *string, clear bool) {
		if clear {
			*field = nil
		} else if v != nil {
			*field = v
		}
	}
	set(&f.Round, c.Round, c.ClearRound)
	set(&f.Source, c.Source, c.ClearSource)
	set(&f.Notes, c.Notes, c.ClearNotes)
	if c.ClearAskedOn {
		f.AskedOn = nil
	} else if c.AskedOn != nil {
		f.AskedOn = c.AskedOn
	}
}

func questionFrom(r store.InterviewQuestion) Question {
	return Question{
		ID: r.ID, JobID: r.JobID, Question: r.Question, Kind: r.Kind, Round: r.Round, Source: r.Source, Notes: r.Notes,
		AskedOn: dateOf(r.AskedOn), Archived: r.ArchivedAt != nil, Version: int(r.Version), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func dateOf(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}
