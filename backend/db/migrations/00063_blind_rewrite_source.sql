-- jobs: an AI rewrite of a blind ad remembers which version of the ad it was
-- made from, so changes to the ad while the AI was working still mark the
-- blind version as out of date. Earlier runs have none (the ad as it is when
-- the answer comes is taken, as before).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_ad_blind_rewrite ADD COLUMN source_hash text CHECK (length(source_hash) <= 64);

-- +goose Down
ALTER TABLE job_ad_blind_rewrite DROP COLUMN IF EXISTS source_hash;
