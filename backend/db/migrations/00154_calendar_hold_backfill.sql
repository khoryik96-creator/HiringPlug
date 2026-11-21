-- +goose Up
-- Seed holds for sources created before calendar jobs covered them.
-- Normal source changes already schedule reconciliation transactionally.
INSERT INTO job (tenant_id, queue, kind, payload)
SELECT i.tenant_id, 'core', 'calendar.source_changed',
       jsonb_build_object('kind', 'interview', 'sourceId', i.id)
FROM interview i
WHERE i.status = 'scheduled'
  AND NOT EXISTS (SELECT 1 FROM calendar_event e WHERE e.tenant_id = i.tenant_id AND e.source = 'interview' AND e.source_id = i.id)
  AND NOT EXISTS (SELECT 1 FROM job j WHERE j.tenant_id = i.tenant_id AND j.kind = 'calendar.source_changed'
                  AND j.status IN ('running','pending') AND j.payload->>'sourceId' = i.id);

INSERT INTO job (tenant_id, queue, kind, payload)
SELECT b.tenant_id, 'core', 'calendar.source_changed',
       jsonb_build_object('kind', 'booking', 'sourceId', b.id)
FROM booking_link b
WHERE b.status = 'booked'
  AND NOT EXISTS (SELECT 1 FROM calendar_event e WHERE e.tenant_id = b.tenant_id AND e.source = 'booking' AND e.source_id = b.id)
  AND NOT EXISTS (SELECT 1 FROM job j WHERE j.tenant_id = b.tenant_id AND j.kind = 'calendar.source_changed'
                  AND j.status IN ('running','pending') AND j.payload->>'sourceId' = b.id);

-- +goose Down
SELECT 1;
