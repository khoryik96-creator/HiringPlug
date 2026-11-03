-- documents module: formatted CVs made by the app also come as a PDF.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE document
  ADD COLUMN pdf_status text CHECK (pdf_status IN ('queued', 'ready', 'failed')),
  ADD COLUMN pdf_key text,
  ADD COLUMN pdf_error text CHECK (length(pdf_error) <= 1000),
  ADD CONSTRAINT document_pdf_ready CHECK ((pdf_status = 'ready') = (pdf_key IS NOT NULL));

-- +goose Down
ALTER TABLE document DROP CONSTRAINT IF EXISTS document_pdf_ready,
  DROP COLUMN IF EXISTS pdf_status, DROP COLUMN IF EXISTS pdf_key, DROP COLUMN IF EXISTS pdf_error;
