-- Personal calendars. No other module queries these tables.
-- name: CalendarConnection :one
SELECT * FROM calendar_connection WHERE user_id= @user_id AND status<>'disconnected';
-- name: CalendarConnectionByID :one
SELECT * FROM calendar_connection WHERE id= @id AND status<>'disconnected';
-- name: LockCalendarUser :exec
SELECT pg_advisory_xact_lock(hashtextextended(app_current_tenant() || ':outlook:' || @user_id::text,0));
-- name: SaveCalendarConnection :one
INSERT INTO calendar_connection(tenant_id,user_id,subject_id,address,app_client_id,calendar_id,calendar_name,teams_available,sealed_tokens)
VALUES(app_current_tenant(),app_current_user(),@subject_id,@address,@app_client_id,@calendar_id,@calendar_name,@teams_available,@sealed_tokens)
ON CONFLICT(tenant_id,user_id) DO UPDATE SET id=new_id('ccn'),version=calendar_connection.version+1,subject_id=excluded.subject_id,address=excluded.address,app_client_id=excluded.app_client_id,calendar_id=excluded.calendar_id,calendar_name=excluded.calendar_name,teams_available=excluded.teams_available,sealed_tokens=excluded.sealed_tokens,status='pending',sync_sequence=1,synced_at=NULL,sync_error=NULL RETURNING *;
-- name: DeleteCalendarConnection :exec
UPDATE calendar_connection SET status='disconnected',sealed_tokens='',sync_sequence=sync_sequence+1 WHERE user_id=app_current_user();
-- name: RefreshCalendarTokens :execrows
UPDATE calendar_connection SET sealed_tokens= @sealed_tokens WHERE id= @id;
-- name: AskOutlookRefresh :one
UPDATE calendar_connection SET sync_sequence=sync_sequence+1,status=CASE WHEN status='reconnect' THEN status ELSE 'pending' END WHERE user_id=app_current_user() AND status<>'disconnected' RETURNING *;
-- name: FinishOutlookRefresh :execrows
UPDATE calendar_connection SET status='ok',synced_at=now(),sync_error=NULL,sync_sequence=sync_sequence+1 WHERE id= @id AND sync_sequence= @sequence;
-- name: FailOutlookRefresh :execrows
UPDATE calendar_connection SET status= @status,sync_error= @sync_error,sync_sequence=sync_sequence+1 WHERE id= @id AND sync_sequence= @sequence;
-- name: SaveCalendarState :exec
INSERT INTO calendar_oauth_state(tenant_id,user_id,state_hash,app_client_id,sealed_verifier,expires_at) VALUES(app_current_tenant(),app_current_user(),@state_hash,@app_client_id,@sealed_verifier,@expires_at)
ON CONFLICT(tenant_id,user_id) DO UPDATE SET state_hash=excluded.state_hash,app_client_id=excluded.app_client_id,sealed_verifier=excluded.sealed_verifier,expires_at=excluded.expires_at,sealed_code=NULL,ticket_hash=NULL,consumed=false;
-- name: ParkCalendarCode :execrows
UPDATE calendar_oauth_state SET consumed=true,sealed_code=sqlc.narg(sealed_code),ticket_hash=sqlc.narg(ticket_hash) WHERE state_hash= @state_hash AND NOT consumed AND expires_at>now();
-- name: ParkedCalendarCode :one
SELECT * FROM calendar_oauth_state WHERE ticket_hash= @ticket_hash AND consumed AND expires_at>now();
-- name: ClaimCalendarCode :execrows
DELETE FROM calendar_oauth_state WHERE state_hash= @state_hash AND consumed AND expires_at>now();
-- name: ClearCalendarState :exec
DELETE FROM calendar_oauth_state WHERE user_id=app_current_user();
-- name: ListCalendarEvents :many
SELECT * FROM calendar_event WHERE user_id=app_current_user() AND (status='active' OR sync_status IN ('pending','failed','conflict')) AND starts_at< @to_time::timestamptz AND ends_at> @from_time::timestamptz ORDER BY starts_at,id LIMIT 1001;
-- name: CalendarEvent :one
SELECT * FROM calendar_event WHERE id= @id;
-- name: InsertCalendarEvent :one
INSERT INTO calendar_event(tenant_id,user_id,title,notes,location,starts_at,ends_at,time_zone,all_day,teams,attendees,source,sync_status)
VALUES(app_current_tenant(),app_current_user(),@title,@notes,@location,@starts_at,@ends_at,@time_zone,@all_day,@teams,@attendees,'local',@sync_status) RETURNING *;
-- name: UpdateCalendarEvent :one
UPDATE calendar_event SET title= @title,notes= @notes,location= @location,starts_at= @starts_at,ends_at= @ends_at,time_zone= @time_zone,all_day= @all_day,teams= @teams,attendees= @attendees,version=version+1,sync_status= @sync_status,sync_error=NULL,updated_at=now()
WHERE id= @id AND version= @expected_version AND source='local' AND status='active' RETURNING *;
-- name: CancelCalendarEvent :one
UPDATE calendar_event SET status='cancelled',version=version+1,sync_status= @sync_status,sync_error=NULL,updated_at=now() WHERE id= @id AND version= @expected_version AND source='local' RETURNING *;
-- name: RetryCalendarEvent :one
UPDATE calendar_event SET sync_status='pending',sync_error=NULL,version=version+1,updated_at=now(),remote_generation=remote_generation+CASE WHEN remote_snapshot->>'deleted'='true' THEN 1 ELSE 0 END,remote_id=CASE WHEN remote_snapshot->>'deleted'='true' THEN NULL ELSE remote_id END,remote_key=CASE WHEN remote_snapshot->>'deleted'='true' THEN NULL ELSE remote_key END,remote_snapshot=CASE WHEN remote_snapshot->>'deleted'='true' THEN NULL ELSE remote_snapshot END WHERE id= @id AND version= @expected_version AND source<>'outlook' RETURNING *;
-- name: PendingCalendarEvents :many
SELECT id FROM calendar_event WHERE user_id= @user_id AND source<>'outlook' AND (status='active' OR remote_id IS NOT NULL) AND sync_status='pending' ORDER BY id;
-- name: SourceCalendarEvents :many
SELECT * FROM calendar_event WHERE source= @source AND source_id= @source_id;
-- name: UpsertCalendarHold :one
INSERT INTO calendar_event(tenant_id,user_id,title,location,starts_at,ends_at,time_zone,source,source_id,source_version,status,sync_status)
VALUES(app_current_tenant(),@user_id,@title,@location,@starts_at,@ends_at,'UTC',@source,@source_id,@source_version,@status,@sync_status)
ON CONFLICT(tenant_id,user_id,source,source_id) DO UPDATE SET title=excluded.title,location=excluded.location,starts_at=excluded.starts_at,ends_at=excluded.ends_at,source_version=excluded.source_version,status=excluded.status,version=calendar_event.version+1,sync_status=excluded.sync_status,sync_error=NULL,updated_at=now()
WHERE calendar_event.source_version<excluded.source_version OR (calendar_event.source_version=excluded.source_version AND calendar_event.status<>excluded.status) RETURNING *;
-- An older version of the source never cancels a hold a newer one made.
-- name: CancelCalendarHold :one
UPDATE calendar_event SET status='cancelled',source_version=GREATEST(source_version,@source_version),version=version+1,sync_status='pending',updated_at=now() WHERE id= @id AND status<>'cancelled' AND source_version<= @source_version RETURNING *;
-- Source jobs for one interview or booking run one at a time.
-- name: LockCalendarSource :exec
SELECT pg_advisory_xact_lock(hashtextextended(app_current_tenant() || ':calendar-source:' || @kind::text || ':' || @source_id::text,0));
-- name: SaveCalendarReceipt :execrows
UPDATE calendar_event SET remote_id=sqlc.narg(remote_id),remote_key=sqlc.narg(remote_key),remote_connection= @connection_id,sent_version= @sent_version,web_url=sqlc.narg(web_url),join_url=sqlc.narg(join_url),remote_snapshot= @remote_snapshot,
 sync_status=CASE WHEN version= @sent_version THEN 'synced' ELSE 'pending' END,sync_error=NULL
WHERE calendar_event.id= @id AND EXISTS(SELECT 1 FROM calendar_connection WHERE calendar_connection.id= @connection_id AND user_id=calendar_event.user_id AND calendar_connection.status<>'disconnected');
-- name: CalendarPushFailed :exec
UPDATE calendar_event SET sync_status='failed',sync_error= @sync_error WHERE id= @id AND version= @expected_version;
-- name: RemoteCalendarEvent :one
SELECT * FROM calendar_event WHERE user_id= @user_id AND remote_connection= @connection_id AND remote_id= @remote_id;
-- name: ImportOutlookEvent :exec
INSERT INTO calendar_event(tenant_id,user_id,title,notes,location,starts_at,ends_at,time_zone,all_day,attendees,source,source_id,status,sync_status,remote_id,remote_connection,remote_key,web_url,join_url)
VALUES(app_current_tenant(),@user_id,@title,@notes,@location,@starts_at,@ends_at,'UTC',@all_day,@attendees,'outlook',@source_id,'active','synced',@remote_id,@connection_id,sqlc.narg(remote_key),sqlc.narg(web_url),sqlc.narg(join_url))
ON CONFLICT(tenant_id,user_id,source,source_id) DO UPDATE SET title=excluded.title,notes=excluded.notes,location=excluded.location,starts_at=excluded.starts_at,ends_at=excluded.ends_at,all_day=excluded.all_day,attendees=excluded.attendees,status='active',remote_key=excluded.remote_key,web_url=excluded.web_url,join_url=excluded.join_url,version=calendar_event.version+1,updated_at=now();
-- name: UpdateManagedFromOutlook :exec
UPDATE calendar_event SET title= @title,notes= @notes,location= @location,starts_at= @starts_at,ends_at= @ends_at,all_day= @all_day,attendees= @attendees,status= @status,remote_key=sqlc.narg(remote_key),web_url=sqlc.narg(web_url),join_url=sqlc.narg(join_url),version=version+1,sent_version=version+1,remote_snapshot= @remote_snapshot,sync_status='synced',sync_error=NULL WHERE id= @id AND version=sent_version;
-- name: MarkCalendarConflict :exec
UPDATE calendar_event SET sync_status='conflict',sync_error='Outlook changed this meeting. Manage its schedule in HiringPlug, or explicitly keep the HiringPlug schedule.',remote_key=sqlc.narg(remote_key),remote_snapshot= @remote_snapshot WHERE id= @id;
-- name: RemoteEventsInWindow :many
SELECT * FROM calendar_event WHERE user_id= @user_id AND remote_connection= @connection_id AND remote_id IS NOT NULL AND starts_at< @to_time::timestamptz AND ends_at> @from_time::timestamptz;
-- name: RemoveImportedCalendarEvent :exec
DELETE FROM calendar_event WHERE id= @id AND source='outlook';
-- name: CancelManagedFromOutlook :exec
UPDATE calendar_event SET status='cancelled',version=version+1,sent_version=version+1,sync_status='synced' WHERE id= @id AND source='local' AND version=sent_version;
-- name: ResetCalendarTargets :exec
UPDATE calendar_event SET remote_id=NULL,remote_key=NULL,remote_connection=NULL,sent_version=0,sync_status=CASE WHEN source='outlook' THEN 'synced' ELSE 'pending' END WHERE user_id=app_current_user();
-- name: ClearImportedEvents :exec
DELETE FROM calendar_event WHERE user_id=app_current_user() AND source='outlook';
-- name: SaveOutlookAvailability :exec
INSERT INTO outlook_availability(tenant_id,user_id,connection_id,status,synced_at,window_start,window_end) VALUES(app_current_tenant(),@user_id,@connection_id,@status,sqlc.narg(synced_at),sqlc.narg(window_start),sqlc.narg(window_end))
ON CONFLICT(tenant_id,user_id) DO UPDATE SET connection_id=excluded.connection_id,status=excluded.status,synced_at=excluded.synced_at,window_start=excluded.window_start,window_end=excluded.window_end;
-- name: ClearOutlookAvailability :exec
DELETE FROM outlook_availability WHERE user_id=app_current_user();
-- name: OutlookHealthForUsers :many
SELECT * FROM outlook_availability WHERE user_id=ANY(@user_ids::text[]);
-- name: ClearOutlookBusy :exec
DELETE FROM outlook_busy WHERE user_id= @user_id AND origin='outlook';
-- name: InsertOutlookBusy :exec
INSERT INTO outlook_busy(tenant_id,user_id,starts_at,ends_at,source,source_id) SELECT app_current_tenant(),@user_id,unnest(@starts::timestamptz[]),unnest(@ends::timestamptz[]),unnest(@sources::text[]),unnest(@source_ids::text[]);
-- Busy times with where they come from (stale interview and booking holds are dropped by the service).
-- name: CalendarBusyRows :many
SELECT user_id,starts_at,ends_at,origin,source,source_id FROM outlook_busy WHERE user_id=ANY(@user_ids::text[]) AND starts_at< @to_time::timestamptz AND ends_at> @from_time::timestamptz AND NOT(source= @except_kind::text AND source_id= @except_id::text) ORDER BY starts_at;
-- name: CurrentCalendarTenant :one
SELECT app_current_tenant()::text;
-- name: RebindCalendarTargets :exec
UPDATE calendar_event SET remote_connection= @connection_id WHERE user_id=app_current_user() AND source<>'outlook' AND remote_connection= @previous_connection;
-- name: BlockReroutedCalendarInvites :exec
UPDATE calendar_event SET sync_status='failed',sync_error='Calendar account changed. Review this appointment before sending invitations from the new account.' WHERE user_id=app_current_user() AND source='local' AND jsonb_array_length(attendees)>0;
-- Network locks live on an isolated worker connection, not a user request.
-- name: LockCalendarNetwork :exec
SELECT pg_advisory_lock(hashtextextended('calendar-network:' || @tenant_id::text || ':' || @user_id::text,0));
-- name: UnlockCalendarNetwork :exec
SELECT pg_advisory_unlock(hashtextextended('calendar-network:' || @tenant_id::text || ':' || @user_id::text,0));

-- name: ClearManagedBusy :exec
DELETE FROM outlook_busy WHERE user_id= @user_id AND origin='local' AND source= @source AND source_id= @source_id;
-- A hold HiringPlug cancelled frees its time at once, its Outlook copy's too (that copy is being deleted).
-- name: ClearSourceBusy :exec
DELETE FROM outlook_busy WHERE user_id= @user_id AND source= @source AND source_id= @source_id;
-- name: SaveManagedBusy :exec
INSERT INTO outlook_busy(tenant_id,user_id,origin,source,source_id,starts_at,ends_at) VALUES(app_current_tenant(),@user_id,'local',@source,@source_id,@starts_at,@ends_at);

-- name: PreviousCalendarConnection :one
SELECT * FROM calendar_connection WHERE user_id= @user_id;

-- name: PrepareCalendarTargets :exec
UPDATE calendar_event SET sync_status='pending' WHERE user_id=app_current_user() AND source<>'outlook' AND sync_status='local';

-- An Outlook event imported before its HiringPlug receipt arrived (the create
-- succeeded but its answer was lost) gives its identity back to the managed row.
-- name: ReleaseImportedRemote :exec
DELETE FROM calendar_event WHERE user_id= @user_id AND remote_connection= @connection_id AND remote_id= @remote_id AND source='outlook';
-- name: ReleaseImportedBusy :exec
DELETE FROM outlook_busy WHERE user_id= @user_id AND origin='outlook' AND source='outlook' AND source_id= @remote_id;
-- HiringPlug rows not yet tied to an event in this connection: one may own an
-- Outlook event its lost receipt never named (matched by transaction ID).
-- name: UnreceiptedCalendarEvents :many
SELECT * FROM calendar_event WHERE user_id= @user_id AND source<>'outlook' AND (remote_id IS NULL OR remote_connection IS DISTINCT FROM @connection_id::text) AND updated_at> now()-interval '120 days';
-- name: AdoptCalendarRemote :exec
UPDATE calendar_event SET remote_id= @remote_id,remote_key=sqlc.narg(remote_key),remote_connection= @connection_id,sync_status='pending',sync_error=NULL WHERE id= @id;
-- Interview and booking holds put into one Outlook connection (taken out of it on disconnect or account change).
-- name: PushedSourceHolds :many
SELECT * FROM calendar_event WHERE user_id= @user_id AND remote_connection= @connection_id AND remote_id IS NOT NULL AND source IN ('interview','booking');
-- name: ForgetPushedSourceHolds :exec
UPDATE calendar_event SET remote_id=NULL,remote_key=NULL,remote_connection=NULL,sent_version=0,remote_snapshot=NULL,sync_status='local',sync_error=NULL WHERE user_id=app_current_user() AND remote_connection= @connection_id AND source IN ('interview','booking');
-- Live interview and booking holds (checked against their sources at each sync).
-- name: ActiveSourceHolds :many
SELECT source,source_id FROM calendar_event WHERE user_id= @user_id AND source IN ('interview','booking') AND status='active' AND ends_at> @from_time::timestamptz AND source_id IS NOT NULL;
