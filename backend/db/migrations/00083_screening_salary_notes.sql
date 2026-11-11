-- Keep salary context and immutable per-call snapshots outside general notes.
-- +goose Up
ALTER TABLE candidate_compensation ADD COLUMN salary_notes text CHECK (length(salary_notes) <= 10000);
CREATE TABLE screening_call_salary (
  call_id text PRIMARY KEY REFERENCES screening_call(id) ON DELETE CASCADE,
  tenant_id text NOT NULL DEFAULT app_current_tenant() REFERENCES tenant(id),
  snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=100000)
);
ALTER TABLE screening_call_salary ENABLE ROW LEVEL SECURITY;
CREATE POLICY salary_snapshot_select ON screening_call_salary FOR SELECT
  USING (tenant_id=app_current_tenant() AND app_has_permission('candidates.compensation:read'));
CREATE POLICY salary_snapshot_insert ON screening_call_salary FOR INSERT
  WITH CHECK (tenant_id=app_current_tenant() AND app_has_permission('candidates.compensation:read'));
-- A recorded call is immutable; express the denial for every command explicitly.
CREATE POLICY salary_snapshot_update ON screening_call_salary FOR UPDATE
  USING (false) WITH CHECK (false);
CREATE POLICY salary_snapshot_delete ON screening_call_salary FOR DELETE
  USING (tenant_id=app_current_tenant() AND app_has_permission('candidates.compensation:update'));
-- Existing salary can be attributed only to the latest call that applied it.
-- Older call values were not stored; do not manufacture historical amounts.
INSERT INTO screening_call_salary(call_id,tenant_id,snapshot)
SELECT c.id,p.tenant_id,jsonb_strip_nulls(jsonb_build_object(
 'currentSalary',CASE WHEN p.current_amount IS NOT NULL THEN jsonb_build_object('amount',p.current_amount::text,'currency',p.current_currency,'period',p.current_period,'description',p.current_description) END,
 'expectedSalary',CASE WHEN p.expected_min IS NOT NULL THEN jsonb_build_object('min',p.expected_min::text,'max',p.expected_max::text,'currency',p.expected_currency,'period',p.expected_period,'description',p.expected_description) END,
 'salaryNotes',p.salary_notes)) || jsonb_build_object(
 'currentSalary',CASE WHEN p.current_amount IS NOT NULL THEN jsonb_strip_nulls(jsonb_build_object('amount',p.current_amount::text,'currency',p.current_currency,'period',p.current_period,'description',p.current_description)) ELSE 'null'::jsonb END,
 'expectedSalary',CASE WHEN p.expected_min IS NOT NULL THEN jsonb_strip_nulls(jsonb_build_object('min',p.expected_min::text,'max',p.expected_max::text,'currency',p.expected_currency,'period',p.expected_period,'description',p.expected_description)) ELSE 'null'::jsonb END)
FROM candidate_compensation p JOIN LATERAL (
 SELECT id,created_at FROM screening_call WHERE candidate_id=p.candidate_id AND salary_recorded ORDER BY called_on DESC,id DESC LIMIT 1
) c ON p.updated_at<=c.created_at;
-- +goose Down
DROP TABLE screening_call_salary;
ALTER TABLE candidate_compensation DROP COLUMN salary_notes;
