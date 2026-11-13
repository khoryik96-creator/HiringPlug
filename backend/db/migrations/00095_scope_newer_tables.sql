-- Tables about candidates added after the access settings (00035) get the
-- same database rule as the older ones: a role limited to its own or its
-- team's candidates reads only rows about candidates it sees. Until now only
-- the application code kept these apart.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE POLICY candidate_scope ON screening_call AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON screening_call_salary AS RESTRICTIVE FOR SELECT
  USING (app_scope_all() OR EXISTS (SELECT 1 FROM screening_call c WHERE c.id = call_id));
CREATE POLICY candidate_scope ON booking_link AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON email_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON industry_assignment AS RESTRICTIVE FOR SELECT
  USING (candidate_id IS NULL OR app_sees_candidate(candidate_id));

-- +goose Down
DROP POLICY IF EXISTS candidate_scope ON industry_assignment;
DROP POLICY IF EXISTS candidate_scope ON email_candidate;
DROP POLICY IF EXISTS candidate_scope ON booking_link;
DROP POLICY IF EXISTS candidate_scope ON screening_call_salary;
DROP POLICY IF EXISTS candidate_scope ON screening_call;
