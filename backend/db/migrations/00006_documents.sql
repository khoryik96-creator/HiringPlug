-- documents module: files attached to candidates (CVs) and their extracted text.

-- +goose Up
CREATE TABLE document (
  id              text PRIMARY KEY DEFAULT new_id('doc') CHECK (id ~ '^doc_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id       text NOT NULL,
  candidate_id    text NOT NULL,
  source          text NOT NULL CHECK (source IN ('jobstreet', 'linkedin', 'original', 'formatted', 'other')),
  file_name       text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  content_type    text NOT NULL,
  size_bytes      bigint NOT NULL CHECK (size_bytes > 0),
  sha256          text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  storage_key     text NOT NULL UNIQUE,
  status          text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'extracted', 'needs_ocr', 'failed')),
  extracted_text  text,
  page_count      integer CHECK (page_count >= 0),
  extractor       text,
  error_message   text,
  -- e.g. "OCR could not read page 3": shown to recruiters so a partial CV is never mistaken for a complete one.
  extraction_warnings text[] NOT NULL DEFAULT '{}',
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX document_candidate_idx ON document (tenant_id, candidate_id, id DESC);

ALTER TABLE document ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON document
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS document;
