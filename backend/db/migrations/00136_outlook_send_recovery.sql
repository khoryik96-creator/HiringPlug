-- +goose Up
-- Retire obsolete review entries left by rescheduling under a new send ID.
-- An existing dispatch or receipt must keep its identity.
WITH ranked AS (
  SELECT id, row_number() OVER (PARTITION BY mailbox_id,remote_id
    ORDER BY (status='needs_review'), updated_at DESC,id DESC) AS position
  FROM email_outlook_send
  WHERE status IN ('scheduled','confirming','uncertain','needs_review')
)
UPDATE email_outlook_send s
SET status='cancelled',version=version+1,updated_at=now(),error='Replaced by a newer schedule.'
FROM ranked r WHERE s.id=r.id AND r.position>1;

-- Bring prior failed jobs into the same visible state as new failures.
UPDATE email_outlook_send s
SET status=CASE WHEN s.status='confirming' THEN 'uncertain' ELSE 'needs_review' END,
    updated_at=now(),
    error=CASE WHEN s.status='confirming' THEN 'HiringPlug could not confirm the send. Check Outlook Drafts and Sent Items.'
               ELSE 'HiringPlug could not process the scheduled send. Review it before scheduling again.' END
WHERE s.status IN ('scheduled','confirming')
  AND EXISTS (SELECT 1 FROM job j WHERE j.tenant_id=s.tenant_id
    AND j.kind='email.outlook_scheduled' AND j.status='dead'
    AND j.payload->>'sendId'=s.id AND j.payload->>'userId'=s.user_id AND j.payload->>'version'=s.version::text);

DROP INDEX email_outlook_one_send;
CREATE UNIQUE INDEX email_outlook_one_send ON email_outlook_send(mailbox_id,remote_id)
 WHERE status IN ('scheduled','confirming','uncertain','needs_review');

-- +goose Down
DROP INDEX email_outlook_one_send;
CREATE UNIQUE INDEX email_outlook_one_send ON email_outlook_send(mailbox_id,remote_id)
 WHERE status IN ('scheduled','confirming','uncertain');
