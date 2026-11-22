-- Queries owned by the notes module. Run inside a tenant-scoped transaction:
-- row-level security limits every query to the current tenant, and sensitive
-- notes to users with notes.sensitive:read.

-- name: InsertNote :one
INSERT INTO note (tenant_id, subject_type, subject_id, kind, body, sensitive, author_id, author_name)
VALUES (app_current_tenant(), @subject_type, @subject_id, @kind, @body, @sensitive, app_current_user(), @author_name)
RETURNING *;

-- name: ListNotes :many
SELECT * FROM note WHERE subject_type = @subject_type AND subject_id = @subject_id ORDER BY id DESC LIMIT 500;

-- name: HiddenSensitiveNotes :one
SELECT app_hidden_sensitive_notes(@subject_type::text, @subject_id::text)::integer AS hidden;

-- name: GetNote :one
SELECT * FROM note WHERE id = @id;

-- name: DeleteNote :execrows
DELETE FROM note WHERE id = @id;

-- name: MoveCandidateNotes :execrows
UPDATE note SET subject_id = @to_id WHERE subject_type = 'candidate' AND subject_id = @from_id;

-- A record deleted for good (the Bin) takes its notes with it.
-- name: DeleteSubjectNotes :exec
DELETE FROM note WHERE subject_type = @subject_type AND subject_id = ANY(@subject_ids::text[]);

-- Merging duplicate companies or contacts: the duplicate's notes go to the
-- record kept, sensitive ones too (app_move_subject_notes, 00164).
-- name: MoveSubjectNotes :exec
SELECT app_move_subject_notes(@subject_type::text, @from_id::text, @to_id::text);

-- Calls logged in a period (reports: each user's activity): screening calls,
-- reference checks and notes of a call.
-- name: CallNotesBetween :many
SELECT id, kind, author_id, created_at FROM note
WHERE kind IN ('screening', 'reference', 'call') AND created_at >= @from_time AND created_at < @to_time
LIMIT 50000;
