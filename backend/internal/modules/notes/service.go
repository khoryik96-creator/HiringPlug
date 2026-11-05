package notes

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const activityLimit = 200

// Service holds the notes use cases.
type Service struct {
	pool     *pgxpool.Pool
	subjects Subjects
	users    Users
}

// Create writes a note on an existing record.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Draft) (Note, error) {
	var out Note
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.createIn(ctx, tx, p, in)
		return err
	})
	return out, err
}

// createIn writes a note in the caller's transaction.
func (s *Service) createIn(ctx context.Context, tx pgx.Tx, p authz.Principal, in Draft) (Note, error) {
	d, errs := Normalize(in, p.Can(api.PermissionNotesSensitiveRead))
	if err := errs.Err(); err != nil {
		return Note{}, err
	}
	if err := s.requireSubject(ctx, tx, d.SubjectType, d.SubjectID); err != nil {
		return Note{}, err
	}
	author := p.Name
	if author == "" {
		author = p.Email
	}
	row, err := store.New(tx).InsertNote(ctx, store.InsertNoteParams{
		SubjectType: d.SubjectType, SubjectID: d.SubjectID, Kind: d.Kind, Body: d.Body, Sensitive: d.Sensitive, AuthorName: author,
	})
	if err != nil {
		return Note{}, err
	}
	// The note's text is not copied into the audit log (it may be sensitive).
	return fromRow(row), audit.Record(ctx, tx, audit.Entry{
		Action: "note.created", EntityType: d.SubjectType, EntityID: d.SubjectID,
		Changes: map[string]any{"noteId": row.ID, "kind": d.Kind, "sensitive": d.Sensitive},
	})
}

// List returns a record's notes the user may read, newest first, and how many
// sensitive ones were left out.
func (s *Service) List(ctx context.Context, p authz.Principal, subjectType, subjectID string) ([]Note, int, error) {
	var out []Note
	var hidden int32
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		// A record the user may not see has no notes for them (and no count of hidden ones).
		if err := s.requireSubject(ctx, tx, subjectType, subjectID); err != nil {
			return err
		}
		q := store.New(tx)
		rows, err := q.ListNotes(ctx, store.ListNotesParams{SubjectType: subjectType, SubjectID: subjectID})
		if err != nil {
			return err
		}
		out = make([]Note, len(rows))
		for i, r := range rows {
			out[i] = fromRow(r)
		}
		hidden, err = q.HiddenSensitiveNotes(ctx, store.HiddenSensitiveNotesParams{SubjectType: subjectType, SubjectID: subjectID})
		return err
	})
	return out, int(hidden), err
}

// Delete removes a note: the author's own, or any with notes:manage.
func (s *Service) Delete(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetNote(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("note")
		}
		if err != nil {
			return err
		}
		n := fromRow(row)
		if !MayDelete(n, p.UserID, p.Can(api.PermissionNotesManage)) {
			return apperr.New(api.ErrorCodeFORBIDDEN, "You can only delete your own notes.")
		}
		if _, err := q.DeleteNote(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "note.deleted", EntityType: n.SubjectType, EntityID: n.SubjectID,
			Changes: map[string]any{"noteId": id, "kind": n.Kind},
		})
	})
}

// Activity returns what changed on a record, newest first, with who did it.
func (s *Service) Activity(ctx context.Context, p authz.Principal, entityType, entityID string) ([]Activity, error) {
	var out []Activity
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		entries, err := audit.List(ctx, tx, entityType, entityID, activityLimit)
		if err != nil {
			return err
		}
		ids := make([]string, len(entries))
		for i, e := range entries {
			ids[i] = e.ActorID
		}
		names, err := s.users.UserNames(ctx, tx, ids)
		if err != nil {
			return err
		}
		out = make([]Activity, len(entries))
		for i, e := range entries {
			name := names[e.ActorID]
			if name == "" {
				name = "System"
			}
			out[i] = Activity{ID: e.ID, Action: e.Action, ActorName: name, Changes: e.Changes, At: e.CreatedAt}
		}
		return nil
	})
	return out, err
}

func (s *Service) requireSubject(ctx context.Context, tx pgx.Tx, subjectType, subjectID string) error {
	exists, known := s.subjects[subjectType]
	var errs apperr.FieldErrors
	if !known {
		errs.Add("subjectType", "notes cannot be written on this kind of record")
		return errs.Err()
	}
	ok, err := exists(ctx, tx, subjectID)
	if err != nil {
		return err
	}
	if !ok {
		errs.Add("subjectId", "no such "+subjectType)
	}
	return errs.Err()
}

func fromRow(r store.Note) Note {
	return Note{
		ID: r.ID, SubjectType: r.SubjectType, SubjectID: r.SubjectID, Kind: r.Kind, Body: r.Body,
		Sensitive: r.Sensitive, AuthorID: r.AuthorID, AuthorName: r.AuthorName, CreatedAt: r.CreatedAt,
	}
}
