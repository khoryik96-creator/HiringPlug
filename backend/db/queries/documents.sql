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
       parse_status, NULL::jsonb AS parsed_profile, parser, pdf_status, pdf_key, pdf_error, parse_error, binned_at
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
UPDATE document SET parse_status = 'queued', parse_error = NULL, updated_at = now() WHERE id = @id RETURNING *;

-- The parser's result. The last result to land is kept (as before the AI
-- use ledger): a reading asked for again while one ran replaces it. A repeated
-- result is counted once in the ledger by its reading's own key.
-- name: SaveParse :one
UPDATE document SET
  parse_status = @parse_status,
  parse_error = sqlc.narg(parse_error),
  parsed_profile = sqlc.narg(parsed_profile),
  parser = sqlc.narg(parser),
  updated_at = now()
WHERE id = @id
RETURNING *;

-- name: MoveDocumentsToCandidate :execrows
UPDATE document SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- name: QueuePdf :exec
UPDATE document SET pdf_status = 'queued', pdf_key = NULL, pdf_error = NULL WHERE id = @id;

-- A copy for a Word document that has none; no row when it has one or one is
-- being made (two asking at once queue it once).
-- name: QueuePdfIfNone :one
UPDATE document SET pdf_status = 'queued' WHERE id = @id AND pdf_status IS NULL RETURNING storage_key;

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

-- The candidate's newest CV (not one the app made, not in the bin), by when it was added.
-- name: NewestCVID :one
SELECT id FROM document
WHERE candidate_id = @candidate_id AND source <> 'formatted' AND binned_at IS NULL
ORDER BY id DESC
LIMIT 1;

-- The candidate's newest CV of some types that they sent (not one the app
-- made, not in the bin): their CV first, then a LinkedIn or JobStreet one,
-- then any other of their files.
-- name: NewestCVOfTypes :one
SELECT id FROM document
WHERE candidate_id = @candidate_id AND source <> 'formatted' AND binned_at IS NULL AND content_type = ANY(@content_types::text[])
ORDER BY CASE source WHEN 'original' THEN 0 WHEN 'linkedin' THEN 1 WHEN 'jobstreet' THEN 1 ELSE 2 END, id DESC
LIMIT 1;

-- A candidate's documents with their text, for a copy of their data.
-- name: ListDocumentTexts :many
SELECT id, source, file_name, status, extracted_text, created_at FROM document
WHERE candidate_id = @candidate_id ORDER BY id DESC LIMIT 200;

-- Possible candidates for a job: those whose CVs best match the job's
-- distinctive terms (weighted words and phrases; each candidate's best CV),
-- best first, leaving out the given candidates. The snippet marks the terms.
-- name: SearchCVs :many
-- Each term counts its weight times how rare it is in the company's CVs (a
-- word nearly every CV has counts for almost nothing). A CV needs two terms,
-- or one of the job's own words or acronyms (weight 2.5 and up), to be a
-- match at all. Its rank is its share of all the terms' worth (0 to 1).
WITH t AS (
  SELECT (@weights::real[])[u.i] AS weight, phraseto_tsquery('english', u.term) AS q
  FROM unnest(@terms::text[]) WITH ORDINALITY AS u(term, i)
), terms AS (
  SELECT q, max(weight) AS weight FROM t WHERE numnode(q) > 0 GROUP BY q
), n AS (
  SELECT greatest(count(*), 1)::real AS total FROM cv_search
), w AS (
  SELECT terms.q, terms.weight, terms.weight * ln(1 + n.total / (1 + (SELECT count(*) FROM cv_search s WHERE s.terms @@ terms.q)))::real AS score
  FROM terms, n
), scored AS (
  SELECT s.candidate_id, s.document_id, sum(w.score)::real AS rank, count(*) AS hits, max(w.weight) AS top
  FROM cv_search s JOIN w ON s.terms @@ w.q
  WHERE NOT (s.candidate_id = ANY (@exclude::text[]))
  GROUP BY s.candidate_id, s.document_id
), best AS (
  SELECT DISTINCT ON (candidate_id) candidate_id, document_id, rank
  FROM scored
  WHERE hits >= 2 OR top >= 2.5
  ORDER BY candidate_id, rank DESC
), q AS (
  SELECT string_agg(terms.q::text, ' | ')::tsquery AS query FROM terms
), worth AS (
  SELECT greatest(sum(score), 0.0001)::real AS total FROM w
)
SELECT b.candidate_id, (b.rank / worth.total)::real AS rank,
       ts_headline('english', d.extracted_text, q.query,
         'MaxFragments=2, MaxWords=14, MinWords=6, FragmentDelimiter=" ... ", StartSel=[[, StopSel=]]')::text AS snippet
FROM best b JOIN document d ON d.id = b.document_id, q, worth
ORDER BY b.rank DESC, b.candidate_id
LIMIT @row_limit;

-- name: RecordAIUse :exec
INSERT INTO document_ai_use (tenant_id, document_id, tokens) VALUES (app_current_tenant(), @document_id, @tokens);

-- Advanced search: the candidates with a CV (read, not formatted) matching
-- a boolean search (as a text search query), up to a limit of CVs. A
-- candidate with two matching CVs comes twice (the caller drops repeats):
-- DISTINCT would read every match before the first one is returned, which
-- for a common word is most of the CVs.
-- name: CandidatesWithMatchingCV :many
SELECT candidate_id::text FROM cv_search
WHERE terms @@ to_tsquery('english', @ts::text)
LIMIT sqlc.narg(row_limit)::int;

-- Where each candidate's best matching CV matches: the words found between
-- [[ and ]].
-- name: CVSnippets :many
WITH q AS (SELECT to_tsquery('english', @ts::text) AS query),
best AS (
  SELECT DISTINCT ON (s.candidate_id) s.candidate_id, s.document_id
  FROM cv_search s, q
  WHERE s.candidate_id = ANY (@candidate_ids::text[]) AND s.terms @@ q.query
  ORDER BY s.candidate_id, ts_rank_cd(s.terms, q.query, 32) DESC, s.document_id DESC
)
SELECT b.candidate_id::text AS candidate_id,
       ts_headline('english', d.extracted_text, q.query,
         'MaxFragments=2, MaxWords=18, MinWords=8, FragmentDelimiter=" ... ", StartSel=[[, StopSel=]]')::text AS snippet
FROM best b JOIN document d ON d.id = b.document_id, q;

-- A document's whole text with a search's words marked (between \x01 and
-- \x02, characters no CV text holds).
-- name: HighlightDocumentText :one
SELECT ts_headline('english', coalesce(extracted_text, ''), to_tsquery('english', @ts::text),
         'HighlightAll=true, StartSel=' || chr(1) || ', StopSel=' || chr(2))::text AS marked
FROM document WHERE id = @id;

-- AI candidate search: the given candidates ordered by how many words their
-- CVs share with a job description (each candidate's best CV), best first.
-- Those whose CVs share none are left out.
-- name: RankCVsByText :many
WITH q AS (
  SELECT string_agg(quote_literal(l), ' | ')::tsquery AS query
  FROM unnest(tsvector_to_array(to_tsvector('english', @job_text::text))) AS l
)
SELECT s.candidate_id::text AS candidate_id, max(ts_rank_cd(s.terms, q.query, 32))::real AS rank
FROM cv_search s, q
WHERE q.query IS NOT NULL AND s.candidate_id = ANY (@candidate_ids::text[]) AND s.terms @@ q.query
GROUP BY s.candidate_id
ORDER BY rank DESC, s.candidate_id
LIMIT @row_limit;

-- A formatted CV deleted (the service checks it is one); its files are
-- removed after the transaction.
-- name: DeleteFormattedDocument :one
DELETE FROM document WHERE id = @id AND source = 'formatted'
RETURNING storage_key, pdf_key;

-- CVs with exactly this content (oldest first), of one candidate or any.
-- name: DocumentsWithContent :many
SELECT id, candidate_id, file_name, sha256, created_at FROM document
WHERE sha256 = ANY(@sums::text[]) AND (sqlc.narg(candidate_id)::text IS NULL OR candidate_id = sqlc.narg(candidate_id)::text)
  AND source <> 'formatted'
ORDER BY created_at;

-- CVs with one of these file names, ignoring case, of one candidate or any.
-- name: DocumentsNamed :many
SELECT id, candidate_id, file_name, sha256, created_at FROM document
WHERE lower(file_name) = ANY(@names::text[]) AND (sqlc.narg(candidate_id)::text IS NULL OR candidate_id = sqlc.narg(candidate_id)::text)
  AND source <> 'formatted'
ORDER BY created_at DESC
LIMIT 500;

-- A candidate deleted for good (the Bin): their documents, and the stored files to remove.
-- name: PurgeCandidateDocuments :many
DELETE FROM document WHERE candidate_id = @candidate_id RETURNING storage_key, pdf_key;

-- A document's candidate, kind and text (for other modules reading a CV just read).
-- Whose a document is and what kind, without its text.
-- name: DocumentOwner :one
SELECT candidate_id, source FROM document WHERE id = @id;

-- name: CVTextOf :one
SELECT candidate_id, source, coalesce(extracted_text, '')::text AS text FROM document WHERE id = @id;

-- name: LockCandidateCVs :exec
SELECT pg_advisory_xact_lock(hashtextextended(app_current_tenant() || ':candidate-cvs:' || @candidate_id::text, 0));

-- name: CVsInCategory :many
SELECT id FROM document WHERE candidate_id = @candidate_id AND binned_at IS NULL
  AND (source = 'formatted') = @formatted::boolean;

-- name: BinDocument :execrows
UPDATE document SET binned_at = now(), updated_at = now() WHERE id = @id AND binned_at IS NULL;

-- name: RestoreBinnedDocument :execrows
UPDATE document SET binned_at = NULL, updated_at = now() WHERE id = @id AND binned_at IS NOT NULL;

-- name: PurgeBinnedDocument :one
DELETE FROM document WHERE id = @id AND binned_at IS NOT NULL RETURNING storage_key,pdf_key;

-- Search by meaning: each candidate's newest CV with text that they sent
-- (their CV first, then a LinkedIn or JobStreet one; not one the app made,
-- not in the bin).
-- Which CV NewestCVTexts gives for each, and which reading of it, without its text.
-- name: NewestCVKeys :many
SELECT DISTINCT ON (candidate_id) candidate_id, id, updated_at
FROM document
WHERE candidate_id = ANY(@candidate_ids::text[]) AND source <> 'formatted' AND binned_at IS NULL
  AND extracted_text IS NOT NULL AND extracted_text <> '' AND source IN ('original', 'linkedin', 'jobstreet')
ORDER BY candidate_id, CASE source WHEN 'original' THEN 0 ELSE 1 END, id DESC;

-- name: NewestCVTexts :many
SELECT DISTINCT ON (candidate_id) candidate_id, id, extracted_text::text AS text
FROM document
WHERE candidate_id = ANY(@candidate_ids::text[]) AND source <> 'formatted' AND binned_at IS NULL
  AND extracted_text IS NOT NULL AND extracted_text <> '' AND source IN ('original', 'linkedin', 'jobstreet')
ORDER BY candidate_id, CASE source WHEN 'original' THEN 0 ELSE 1 END, id DESC;

-- Whether the transaction's user may see salaries (a formatted CV made with
-- the salary is only theirs to open).
-- name: MaySeeSalary :one
SELECT app_has_permission('candidates.compensation:read')::boolean AS may;
