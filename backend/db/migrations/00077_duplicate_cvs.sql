-- documents: find a CV by its content (sha256) or its file name quickly, so
--   the same CV is never uploaded twice.
-- intake: an imported file that is exactly a CV already in HiringPlug is
--   skipped (status duplicate), its candidate_id the one who has it.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE INDEX document_sha256_idx ON document (tenant_id, sha256);
CREATE INDEX document_file_name_idx ON document (tenant_id, lower(file_name));
ALTER TABLE cv_import_item DROP CONSTRAINT cv_import_item_status_check;
ALTER TABLE cv_import_item ADD CONSTRAINT cv_import_item_status_check
  CHECK (status IN ('reading', 'created', 'added', 'unread', 'failed', 'duplicate'));

-- +goose Down
UPDATE cv_import_item SET status = 'failed' WHERE status = 'duplicate';
ALTER TABLE cv_import_item DROP CONSTRAINT cv_import_item_status_check;
ALTER TABLE cv_import_item ADD CONSTRAINT cv_import_item_status_check
  CHECK (status IN ('reading', 'created', 'added', 'unread', 'failed'));
DROP INDEX IF EXISTS document_file_name_idx;
DROP INDEX IF EXISTS document_sha256_idx;
