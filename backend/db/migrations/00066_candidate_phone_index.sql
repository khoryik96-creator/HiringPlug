-- candidates: phone numbers are looked up whole (E.164, as stored), not by
-- their last digits, so an ordinary index finds them.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE INDEX candidate_tenant_phone_idx ON candidate (tenant_id, phone) WHERE phone IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS candidate_tenant_phone_idx;
