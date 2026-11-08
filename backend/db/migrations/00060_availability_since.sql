-- candidates: when a candidate's status (looking out, passive, not looking,
-- placed) was last set, so the page can say "Not looking since 3 Nov (5
-- days)". Set by the update when the status changes. Statuses set before
-- this take the time of their last change in the audit log, or else when the
-- candidate was added.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate ADD COLUMN availability_since timestamptz;

UPDATE candidate c
   SET availability_since = coalesce(
         (SELECT max(a.created_at) FROM audit_log a
           WHERE a.tenant_id = c.tenant_id AND a.entity_type = 'candidate' AND a.entity_id = c.id
             AND a.changes ? 'availability'),
         c.created_at)
 WHERE c.availability IS NOT NULL;

-- +goose Down
ALTER TABLE candidate DROP COLUMN IF EXISTS availability_since;
