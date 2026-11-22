-- name: OutlookSync :one
SELECT * FROM email_outlook_sync WHERE mailbox_id = @mailbox_id;

-- name: RequestOutlookSync :one
INSERT INTO email_outlook_sync(mailbox_id,generation) VALUES (@mailbox_id,@generation)
ON CONFLICT (mailbox_id) DO UPDATE SET generation=excluded.generation, requested_at=now(), sync_error=NULL
WHERE email_outlook_sync.requested_at < now()-interval '20 seconds'
RETURNING *;

-- name: RemoveOutlookSync :exec
DELETE FROM email_outlook_sync WHERE user_id=app_current_user();

-- name: MarkOutlookSync :exec
UPDATE email_outlook_sync SET generation= @generation,last_sync_at=COALESCE(sqlc.narg(last_sync_at),last_sync_at),sync_error=sqlc.narg(sync_error)
WHERE mailbox_id= @mailbox_id;

-- name: MarkOutlookFolders :exec
UPDATE email_outlook_sync SET folders_at=now() WHERE mailbox_id = @mailbox_id;

-- name: SaveOutlookFolder :exec
INSERT INTO email_outlook_folder(mailbox_id,remote_id,name,kind,parent_id)
VALUES (@mailbox_id,@remote_id,@name,@kind,sqlc.narg(parent_id))
ON CONFLICT(mailbox_id,remote_id) DO UPDATE SET name=excluded.name,kind=excluded.kind,parent_id=excluded.parent_id;

-- name: RemoveMissingOutlookFolders :exec
DELETE FROM email_outlook_folder WHERE mailbox_id= @mailbox_id AND NOT(remote_id=ANY(@remote_ids::text[]));

-- name: OutlookFolders :many
SELECT * FROM email_outlook_folder WHERE mailbox_id= @mailbox_id
ORDER BY CASE kind WHEN 'inbox' THEN 0 WHEN 'drafts' THEN 1 WHEN 'sentitems' THEN 2 WHEN 'deleteditems' THEN 3 WHEN 'archive' THEN 4 WHEN 'junkemail' THEN 5 ELSE 6 END,name,id;

-- name: OutlookUnreadCounts :many
SELECT folder_id, count(*)::int AS unread FROM email_outlook_message
WHERE mailbox_id= @mailbox_id AND NOT is_read AND NOT is_draft GROUP BY folder_id;

-- name: OutlookDueFolders :many
SELECT * FROM email_outlook_folder WHERE mailbox_id= @mailbox_id
  AND (checked_at IS NULL OR checked_at < now() - CASE WHEN complete THEN interval '2 minutes' ELSE interval '15 seconds' END)
-- Unchecked folders first; among them Outlook's own, the Inbox first.
ORDER BY checked_at NULLS FIRST,CASE kind WHEN 'inbox' THEN 0 WHEN 'drafts' THEN 1 WHEN 'sentitems' THEN 2 WHEN 'archive' THEN 3 WHEN 'deleteditems' THEN 4 WHEN 'junkemail' THEN 5 ELSE 6 END,id LIMIT 4;

-- name: OutlookFolder :one
SELECT * FROM email_outlook_folder WHERE id= @id AND mailbox_id= @mailbox_id;

-- name: SaveOutlookCursor :exec
UPDATE email_outlook_folder SET cursor_url=sqlc.narg(cursor_url),complete= @complete,checked_at=sqlc.narg(checked_at)
WHERE id= @id;

-- name: ClearOutlookFolder :exec
DELETE FROM email_outlook_message WHERE folder_id= @folder_id;

-- name: SaveOutlookMessage :exec
INSERT INTO email_outlook_message(mailbox_id,folder_id,remote_id,conversation,subject,sender,participants,received_at,changed_at,is_read,is_draft,has_attachments,web_url,sender_name,flagged)
VALUES (@mailbox_id,@folder_id,@remote_id,@conversation,@subject,@sender,@participants,@received_at,@changed_at,@is_read,@is_draft,@has_attachments,sqlc.narg(web_url),@sender_name,@flagged)
ON CONFLICT(mailbox_id,remote_id) DO UPDATE SET folder_id=excluded.folder_id,conversation=excluded.conversation,subject=excluded.subject,sender=excluded.sender,
participants=excluded.participants,received_at=excluded.received_at,changed_at=excluded.changed_at,
is_read=excluded.is_read,is_draft=excluded.is_draft,has_attachments=excluded.has_attachments,web_url=excluded.web_url,
sender_name=excluded.sender_name,flagged=excluded.flagged
WHERE email_outlook_message.changed_at<=excluded.changed_at;

-- name: RemoveOutlookMessage :exec
DELETE FROM email_outlook_message WHERE folder_id= @folder_id AND remote_id= @remote_id;

-- name: OutlookMessage :one
SELECT * FROM email_outlook_message WHERE id= @id AND mailbox_id= @mailbox_id;

-- name: ListOutlookMessages :many
SELECT m.* FROM email_outlook_message m
LEFT JOIN email_outlook_link l ON l.mailbox_id=m.mailbox_id AND l.conversation=m.conversation
WHERE m.mailbox_id= @mailbox_id
 AND (@folder::text='' OR m.folder_id= @folder)
 AND (@thread::text='' OR m.conversation= @thread)
 AND (@candidate::text='' OR l.candidate_id= @candidate)
 AND (@contact::text='' OR l.contact_id= @contact)
 AND (@search::text='' OR strpos(lower(m.subject),lower(@search))>0 OR strpos(lower(m.sender_name),lower(@search))>0 OR lower(@search)=ANY(m.participants))
ORDER BY m.received_at DESC,m.id DESC LIMIT 51 OFFSET @skip::integer;

-- name: SetOutlookRead :exec
UPDATE email_outlook_message SET is_read= @is_read,changed_at=GREATEST(changed_at,@changed_at) WHERE id= @id;

-- name: SetOutlookFlag :exec
UPDATE email_outlook_message SET flagged= @flagged,changed_at=GREATEST(changed_at,@changed_at) WHERE id= @id;

-- name: MoveOutlookMessage :exec
UPDATE email_outlook_message SET folder_id= @folder_id,changed_at=GREATEST(changed_at,@changed_at) WHERE id= @id;

-- name: SaveOutlookLink :exec
INSERT INTO email_outlook_link(mailbox_id,conversation,candidate_id,contact_id)
VALUES (@mailbox_id,@conversation,sqlc.narg(candidate_id),sqlc.narg(contact_id))
ON CONFLICT(mailbox_id,conversation) DO UPDATE SET candidate_id=excluded.candidate_id,contact_id=excluded.contact_id;

-- name: RemoveOutlookLink :exec
DELETE FROM email_outlook_link WHERE mailbox_id= @mailbox_id AND conversation= @conversation;

-- name: OutlookLink :one
SELECT * FROM email_outlook_link WHERE mailbox_id= @mailbox_id AND conversation= @conversation;

-- name: OutlookThreadHasAddress :one
SELECT EXISTS(SELECT 1 FROM email_outlook_message WHERE mailbox_id= @mailbox_id AND conversation= @conversation AND @address::text=ANY(participants));


-- name: NudgeOutlookFolders :exec
UPDATE email_outlook_folder SET checked_at=NULL WHERE mailbox_id = @mailbox_id;

-- name: OutlookDraftFolder :one
SELECT * FROM email_outlook_folder WHERE mailbox_id = @mailbox_id AND kind='drafts';

-- name: OutlookByRemoteID :one
SELECT * FROM email_outlook_message WHERE mailbox_id = @mailbox_id AND remote_id = @remote_id;

-- name: OutlookScheduled :one
SELECT * FROM email_outlook_send WHERE id = @id;

-- name: OutlookActiveSend :one
SELECT * FROM email_outlook_send WHERE mailbox_id = @mailbox_id AND remote_id = @remote_id
 AND status IN ('scheduled','confirming','uncertain','needs_review');

-- name: OutlookSendForDraftIndex :one
SELECT * FROM email_outlook_send WHERE mailbox_id = @mailbox_id AND message_id = @message_id
 AND status IN ('scheduled','needs_review');

-- name: OutlookRecentSends :many
SELECT * FROM email_outlook_send WHERE user_id=app_current_user()
ORDER BY CASE status WHEN 'scheduled' THEN 0 WHEN 'confirming' THEN 1 WHEN 'needs_review' THEN 2 WHEN 'uncertain' THEN 3 ELSE 4 END,
 CASE WHEN status='scheduled' THEN send_at END ASC, updated_at DESC,id LIMIT 51 OFFSET @skip::integer;

-- name: CreateOutlookScheduled :one
INSERT INTO email_outlook_send(mailbox_id,message_id,remote_id,draft_version,candidate_id,subject,send_at)
VALUES (@mailbox_id,@message_id,@remote_id,@draft_version,sqlc.narg(candidate_id),@subject,@send_at)
RETURNING *;

-- name: RescheduleOutlookSend :one
UPDATE email_outlook_send SET send_at= @send_at, version=version+1,status='scheduled',
 message_id= @message_id,draft_version= @draft_version,subject= @subject,
 candidate_id=sqlc.narg(candidate_id),updated_at=now(),error=NULL,attempted_at=NULL
WHERE id= @id AND status IN ('scheduled','needs_review') AND version= @version RETURNING *;

-- name: ReviewOutlookScheduled :exec
UPDATE email_outlook_send SET draft_version= @draft_version,subject= @subject,updated_at=now()
WHERE id= @id AND status='scheduled' AND draft_version= @previous_draft_version;

-- name: CancelOutlookSend :one
UPDATE email_outlook_send SET status='cancelled',version=version+1,updated_at=now(),error=NULL
WHERE id= @id AND status IN ('scheduled','needs_review') AND version= @version RETURNING *;

-- name: CancelOutlookSendsForMailbox :exec
UPDATE email_outlook_send SET status='cancelled',version=version+1,updated_at=now(),error='The Outlook mailbox was disconnected or replaced.'
WHERE user_id=app_current_user() AND status='scheduled';

-- name: ClaimOutlookSend :execrows
UPDATE email_outlook_send SET status='confirming',attempted_at=now(),updated_at=now(),error=NULL
WHERE id= @id AND status='scheduled' AND version= @version;

-- name: FinishOutlookSend :execrows
UPDATE email_outlook_send SET status= @status,error=sqlc.narg(error),updated_at=now()
WHERE id= @id AND version= @version AND status= @previous_status;

-- name: ResolveOutlookSend :one
UPDATE email_outlook_send SET status= @status,error=sqlc.narg(error),version=version+1,updated_at=now()
WHERE id= @id AND version= @version AND status='uncertain'
RETURNING *;

-- name: TryLockOutlookSendMailbox :one
SELECT pg_try_advisory_lock(hashtextextended(@tenant_id::text || ':' || @user_id::text,0)) AS acquired;

-- name: MoveOutlookLinksToContact :exec
UPDATE email_outlook_link SET contact_id = @to_id WHERE contact_id = @from_id;
