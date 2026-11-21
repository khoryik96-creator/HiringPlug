-- +goose Up
ALTER TABLE document ADD COLUMN binned_at timestamptz;
ALTER TABLE studio_cv ADD COLUMN replace_document_ids text[] NOT NULL DEFAULT '{}';
ALTER TABLE bin_entry DROP CONSTRAINT bin_entry_kind_check;
ALTER TABLE bin_entry ADD CONSTRAINT bin_entry_kind_check CHECK (kind IN ('candidate', 'contact', 'company', 'document'));

CREATE POLICY bin_hidden ON document AS RESTRICTIVE FOR SELECT
  USING (binned_at IS NULL OR app_show_binned());
CREATE INDEX document_active_category_idx ON document (tenant_id, candidate_id, (source = 'formatted'))
  WHERE binned_at IS NULL;

DROP TRIGGER document_cv_search ON document;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION cv_search_sync() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM cv_search WHERE document_id = NEW.id;
  IF NEW.binned_at IS NULL AND NEW.source <> 'formatted'
     AND NEW.status = 'extracted' AND NEW.extracted_text IS NOT NULL THEN
    INSERT INTO cv_search (document_id, tenant_id, candidate_id, terms)
    VALUES (NEW.id, NEW.tenant_id, NEW.candidate_id, to_tsvector('english', NEW.extracted_text));
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER document_cv_search AFTER INSERT OR UPDATE OF extracted_text, candidate_id, source, status, binned_at ON document
  FOR EACH ROW EXECUTE FUNCTION cv_search_sync();

-- +goose Down
DROP TRIGGER document_cv_search ON document;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION cv_search_sync() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM cv_search WHERE document_id = NEW.id;
  IF NEW.source <> 'formatted' AND NEW.status = 'extracted' AND NEW.extracted_text IS NOT NULL THEN
    INSERT INTO cv_search (document_id, tenant_id, candidate_id, terms)
    VALUES (NEW.id, NEW.tenant_id, NEW.candidate_id, to_tsvector('english', NEW.extracted_text));
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER document_cv_search AFTER INSERT OR UPDATE OF extracted_text, candidate_id, source, status ON document
  FOR EACH ROW EXECUTE FUNCTION cv_search_sync();
DROP POLICY bin_hidden ON document;
INSERT INTO cv_search (document_id, tenant_id, candidate_id, terms)
SELECT id, tenant_id, candidate_id, to_tsvector('english', extracted_text) FROM document
WHERE binned_at IS NOT NULL AND source <> 'formatted' AND status = 'extracted' AND extracted_text IS NOT NULL
ON CONFLICT (document_id) DO NOTHING;
DROP INDEX document_active_category_idx;
ALTER TABLE document DROP COLUMN binned_at;
ALTER TABLE studio_cv DROP COLUMN replace_document_ids;
DELETE FROM bin_entry WHERE kind = 'document';
ALTER TABLE bin_entry DROP CONSTRAINT bin_entry_kind_check;
ALTER TABLE bin_entry ADD CONSTRAINT bin_entry_kind_check CHECK (kind IN ('candidate', 'contact', 'company'));
