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
SELECT * FROM email_outlook_folder WHERE mailbox_id= @mailbox_id ORDER BY CASE kind WHEN 'inbox' THEN 0 WHEN 'sentitems' THEN 1 WHEN 'drafts' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END,name,id;

-- name: OutlookDueFolders :many
SELECT * FROM email_outlook_folder WHERE mailbox_id= @mailbox_id
  AND (checked_at IS NULL OR checked_at < now() - CASE WHEN complete THEN interval '2 minutes' ELSE interval '15 seconds' END)
ORDER BY checked_at NULLS FIRST,id LIMIT 4;

-- name: OutlookFolder :one
SELECT * FROM email_outlook_folder WHERE id= @id AND mailbox_id= @mailbox_id;

-- name: SaveOutlookCursor :exec
UPDATE email_outlook_folder SET cursor_url=sqlc.narg(cursor_url),complete= @complete,checked_at=sqlc.narg(checked_at)
WHERE id= @id;

-- name: ClearOutlookFolder :exec
DELETE FROM email_outlook_message WHERE folder_id= @folder_id;

-- name: SaveOutlookMessage :exec
INSERT INTO email_outlook_message(mailbox_id,folder_id,remote_id,conversation,subject,sender,participants,received_at,changed_at,is_read,is_draft,has_attachments,web_url)
VALUES (@mailbox_id,@folder_id,@remote_id,@conversation,@subject,@sender,@participants,@received_at,@changed_at,@is_read,@is_draft,@has_attachments,sqlc.narg(web_url))
ON CONFLICT(mailbox_id,remote_id) DO UPDATE SET folder_id=excluded.folder_id,conversation=excluded.conversation,subject=excluded.subject,sender=excluded.sender,
participants=excluded.participants,received_at=excluded.received_at,changed_at=excluded.changed_at,
is_read=excluded.is_read,is_draft=excluded.is_draft,has_attachments=excluded.has_attachments,web_url=excluded.web_url
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
 AND (@search::text='' OR strpos(lower(m.subject),lower(@search))>0 OR lower(@search)=ANY(m.participants))
ORDER BY m.received_at DESC,m.id DESC LIMIT 51 OFFSET @skip::integer;

-- name: SetOutlookRead :exec
UPDATE email_outlook_message SET is_read= @is_read,changed_at=GREATEST(changed_at,@changed_at) WHERE id= @id;

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
