-- A binned candidate's (or binned company's) interviews and offers were still
-- listed, with no candidate or job: their visibility check is skipped for
-- users who see every candidate, and nothing tied them to the Bin. Now an
-- interview or an offer is visible only while its application is (the
-- application's own policies hide it in the Bin, or out of the user's scope),
-- so reminders and lists leave them out too, and a restore brings them back.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE POLICY bin_hidden ON interview AS RESTRICTIVE FOR SELECT
  USING (EXISTS (SELECT 1 FROM application a WHERE a.id = interview.application_id));
CREATE POLICY bin_hidden ON offer AS RESTRICTIVE FOR SELECT
  USING (EXISTS (SELECT 1 FROM application a WHERE a.id = offer.application_id));

-- +goose Down
DROP POLICY IF EXISTS bin_hidden ON offer;
DROP POLICY IF EXISTS bin_hidden ON interview;
