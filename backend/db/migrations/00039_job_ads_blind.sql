-- jobs: a job's country and official intake date; each ad's country, and its
-- blind version. The blind text is made from the ad when it is read; only a
-- person's own blind text is stored (blind_*_edit), with a hash of the ad it
-- was written for, so the ad changing afterwards is noticed. publish_blind:
-- the careers page shows the blind version (new ads; ads already published
-- keep showing what they showed).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_order
  ADD COLUMN country text CHECK (country ~ '^[A-Z]{2}$'),
  ADD COLUMN intake_date date;
UPDATE job_order SET intake_date = created_at::date;
ALTER TABLE job_order ALTER COLUMN intake_date SET NOT NULL, ALTER COLUMN intake_date SET DEFAULT current_date;

ALTER TABLE job_ad
  ADD COLUMN country text CHECK (country ~ '^[A-Z]{2}$'),
  ADD COLUMN client_description text CHECK (length(client_description) <= 200),
  ADD COLUMN blind_title_edit text CHECK (length(btrim(blind_title_edit)) BETWEEN 1 AND 200),
  ADD COLUMN blind_description_edit text CHECK (length(blind_description_edit) <= 20000),
  ADD COLUMN blind_edited boolean NOT NULL DEFAULT false,
  ADD COLUMN blind_source_hash text CHECK (length(blind_source_hash) <= 64),
  ADD COLUMN publish_blind boolean NOT NULL DEFAULT false;
ALTER TABLE job_ad ALTER COLUMN publish_blind SET DEFAULT true;

-- +goose Down
ALTER TABLE job_ad DROP COLUMN IF EXISTS country, DROP COLUMN IF EXISTS client_description, DROP COLUMN IF EXISTS blind_title_edit,
  DROP COLUMN IF EXISTS blind_description_edit, DROP COLUMN IF EXISTS blind_edited, DROP COLUMN IF EXISTS blind_source_hash,
  DROP COLUMN IF EXISTS publish_blind;
ALTER TABLE job_order DROP COLUMN IF EXISTS country, DROP COLUMN IF EXISTS intake_date;
