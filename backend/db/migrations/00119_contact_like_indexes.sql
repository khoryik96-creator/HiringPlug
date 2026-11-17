-- contacts module: Find HR shows who may be a contact already, by LinkedIn
-- profile or by name (ContactsLike); these indexes answer both without
-- reading every contact.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE INDEX contact_linkedin_url_idx ON contact (linkedin_url) WHERE linkedin_url IS NOT NULL;
CREATE INDEX contact_full_name_idx ON contact (lower(btrim(first_name || ' ' || last_name)));

-- +goose Down
DROP INDEX IF EXISTS contact_full_name_idx;
DROP INDEX IF EXISTS contact_linkedin_url_idx;
