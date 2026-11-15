-- A placement can be "backed out": the candidate changed their mind or the
-- client withdrew the offer after it was recorded. The sheet stays (what was
-- agreed, for the record); its fee no longer counts, like one that did not
-- start. It is set when a placed candidate is moved back to offer or to
-- withdrawn, and placing them again makes it active again.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE placement DROP CONSTRAINT placement_status_check;
ALTER TABLE placement ADD CONSTRAINT placement_status_check
  CHECK (status IN ('active', 'fell_through', 'cancelled', 'backed_out'));

-- +goose Down
UPDATE placement SET status = 'cancelled' WHERE status = 'backed_out';
ALTER TABLE placement DROP CONSTRAINT placement_status_check;
ALTER TABLE placement ADD CONSTRAINT placement_status_check
  CHECK (status IN ('active', 'fell_through', 'cancelled'));
