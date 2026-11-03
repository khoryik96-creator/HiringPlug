-- Queries owned by the documents module. Run inside a tenant-scoped transaction.

-- name: NewDocumentID :one
SELECT new_id('doc')::text AS id;

-- name: InsertDocument :one
INSERT INTO document (id, tenant_id, candidate_id, source, file_name, content_type, size_bytes, sha256, storage_key, created_by)
VALUES (@id, app_current_tenant(), @candidate_id, @source, @file_name, @content_type, @size_bytes, @sha256, @storage_key, app_current_user())
RETURNING *;

-- name: GetDocument :one
SELECT * FROM document WHERE id = @id;

-- name: ListDocumentsForCandidate :many
SELECT id, tenant_id, candidate_id, source, file_name, content_type, size_bytes, sha256, storage_key,
       status, NULL::text AS extracted_text, page_count, extractor, error_message, extraction_warnings, created_by, created_at, updated_at,
       parse_status, NULL::jsonb AS parsed_profile, parser, pdf_status, pdf_key, pdf_error
FROM document WHERE candidate_id = @candidate_id
ORDER BY id DESC;

-- name: SaveExtraction :one
UPDATE document SET
  status = @status,
  extracted_text = sqlc.narg(extracted_text),
  page_count = sqlc.narg(page_count),
  extractor = @extractor,
  error_message = sqlc.narg(error_message),
  extraction_warnings = @extraction_warnings,
  updated_at = now()
WHERE id = @id
RETURNING *;

-- name: QueueParse :one
UPDATE document SET parse_status = 'queued', updated_at = now() WHERE id = @id RETURNING *;

-- name: SaveParse :one
UPDATE document SET
  parse_status = @parse_status,
  parsed_profile = sqlc.narg(parsed_profile),
  parser = sqlc.narg(parser),
  updated_at = now()
WHERE id = @id
RETURNING *;

-- name: MoveDocumentsToCandidate :execrows
UPDATE document SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- name: QueuePdf :exec
UPDATE document SET pdf_status = 'queued', pdf_key = NULL, pdf_error = NULL WHERE id = @id;

-- name: SavePdf :one
UPDATE document SET pdf_status = @pdf_status, pdf_key = sqlc.narg(pdf_key), pdf_error = sqlc.narg(pdf_error)
WHERE id = @id AND pdf_status = 'queued'
RETURNING id;

-- The text of the candidate's newest CV that was read (not one the app made).
-- name: LatestCvText :one
SELECT extracted_text::text FROM document
WHERE candidate_id = @candidate_id AND source <> 'formatted' AND extracted_text IS NOT NULL AND extracted_text <> ''
ORDER BY id DESC
LIMIT 1;
