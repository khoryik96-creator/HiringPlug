# Outlook calendar

Open the calendar icon beside **Settings** to view your private calendar.
Month, Week, Day and Agenda show local appointments, Outlook events and the
person's HiringPlug interviews and booked calls. Email connections continue to
work independently. **Day** and **Week** show hourly slots in your browser's
time zone, with all-day events above them. Click an empty hour to start an
appointment.

## Connect Microsoft

1. An owner/admin configures the Microsoft app in **Settings > Email connection**,
   or the deployment supplies the existing `HP_MAIL_MICROSOFT_CLIENT_ID` and
   `HP_MAIL_MICROSOFT_CLIENT_SECRET`. Secure storage uses `HP_MAIL_KEYS_SECRET`;
   keep it across updates and backups.
2. In that Entra app, add a **Web** redirect URI:
   `<HP_PUBLIC_URL>/v1/calendar/callback`. For the local Docker deployment this is
   `http://localhost:3000/v1/calendar/callback`. Keep the existing mailbox
   callback too. Proxies must route `/v1/calendar/callback` to the API without
   logging its query string (included in the shipped nginx config).
3. Add Microsoft Graph **delegated** `Calendars.ReadWrite`, alongside `User.Read`.
   Sign-in requests `openid offline_access User.Read Calendars.ReadWrite`.
   Grant admin consent if the organization requires it. These are user calendar
   permissions, not organization-wide application permissions.
4. Each person opens Calendar and clicks **Connect Outlook**, approves access
   and returns to their original signed-in HiringPlug account. Calendar consent
   is separate from mailbox consent. Existing email connections need no change.

The connection uses the Microsoft account's editable **default calendar**.
Microsoft 365 and Outlook.com accounts are supported when the registered app's
account types and the organization's policy allow them. Teams is offered only
when that calendar advertises Teams support. Other calendar providers and
shared/delegated calendars are not included.

## Scheduling behavior

- Outlook refreshes every five minutes, covering 31 days before the current UTC
  day through 94 days after it. Manual Refresh queues another read. The screen
  labels this window and warns when navigating beyond it. A full Outlook history
  import and webhook subscriptions are not part of this integration.
- Outlook events are read-only in HiringPlug; **Open in Outlook** opens them at
  their source. Recurring occurrences come from Microsoft's calendarView.
- Own HiringPlug appointments can be created, edited and cancelled. Adding
  attendees explicitly sends Microsoft invitations; changing or cancelling them
  sends updates/cancellations. The form names these actions. Ordinary appointments
  have no attendees. Local appointments work without a Microsoft connection.
- Interviews and candidate-booked calls produce private Outlook holds for each
  active participating HiringPlug user with calendar access. Moves, cancellations
  and panel changes update/remove those holds. Holds never invite candidates.
  The existing candidate ICS/email invitation flow remains. A booking host whose
  Outlook is working receives the native hold instead of another ICS email copy;
  while their connection is failing or needs reconnecting they are emailed.
- On a scheduled interview, open **Outlook invitation** and check the candidate's
  address before sending. The organizer can add a Teams meeting for a video
  interview when the connected default calendar supports Teams. The reminder
  choice sets the Outlook event reminder. Each attendee's calendar keeps its
  own notification settings.
- HiringPlug updates that invitation when the interview moves and asks Outlook
  to cancel it when the interview is cancelled. The original organizer and
  candidate address are pinned. A disconnected or switched account, a changed
  candidate address or withdrawn consent requires attention. In those cases,
  review the original event in Outlook before taking action. Private panel
  holds still apply, and the organizer's native event replaces their hold once
  Outlook has accepted it. **Join Teams** appears on the interview when
  Microsoft returns the link. The panel's day-before email includes it if the
  link is available when the reminder runs.
- **Outlook invitation** shows the candidate's latest reply from the organizer's
  calendar sync (a reply Outlook reports that HiringPlug does not know leaves
  the last one shown). **Email candidate a day before** opts into a separate
  reminder, sent as the organizer's email (from their connected mailbox, else
  the configured mail server; replies go to them) and listed on the candidate's
  Emails tab. The organizer can turn it off
  from the invitation panel. Moving an interview moves the reminder. At delivery
  HiringPlug checks the latest schedule, organizer, address, known RSVP, and
  consent. If an interview is first invited within 24 hours, the email can go
  once Outlook confirms the invitation, while the interview is over an hour
  away. The reminder includes the Teams link when available. A reminder no
  longer due is dropped, not listed as a failed email.
- Deleting a candidate for good (or their pipeline entry) cancels their Outlook
  invitations, and removes HiringPlug's copy (it holds the candidate's address)
  once Outlook confirms. A conflict stays marked once; a sync with nothing new
  in Outlook does not mark it again.
- An Outlook change to an own appointment is reflected on the next refresh.
  A changed/deleted interview or booking hold is flagged for review; its source
  schedule stays in HiringPlug. **Keep HiringPlug details & sync** explicitly
  resolves a conflict. Changes should otherwise be made in the original interview
  or booking link. Concurrent edits use versions and Outlook conditional writes.
- Booking pages and interview clash checks use Outlook busy intervals. While a
  sync is late, failing or needs reconnecting, the last known busy times still
  count and nothing is blocked: a candidate's booking page keeps working, and
  scheduling an interview names the interviewer whose Outlook "has not synced
  lately" beside other clashes (other interviews, by name; calendar
  appointments, by owner only). An interview the scheduler may not see (another
  recruiter's candidate) still counts, as a calendar appointment. A calendar
  connected a moment ago, its first sync on its way, is not "not synced
  lately". Every clash can be scheduled over. The optional
  ICS feed counts again while Outlook is not working; native Outlook takes
  precedence while it is.
- An appointment Outlook has, marked Free there, does not block the time: once
  Outlook shows it, Outlook's busy time governs (the interval kept while it was
  on its way is dropped). Interview and booking holds always block, until
  HiringPlug takes someone off them: then they are free at once, even while
  Outlook has not deleted its copy yet.
- A hold whose interview or booking is gone (deleted with its candidate, or a
  link no longer booked) never blocks a time, is hidden from the calendar, and
  is cancelled and taken out of Outlook (checked when read and at each sync).
  Whether it is gone is checked as the system, not as the reader, so an
  interview the reader may not see is not mistaken for a deleted one. It is
  cancelled the first time it is read, in that same transaction, with no job
  queued per view.
- Source jobs for one interview or booking run one at a time, and a hold only
  moves forward in its source's version: an older job never cancels or brings
  back what a newer one decided.

Disconnect deletes from Outlook the interview and booking holds HiringPlug put
there (nothing would update them afterwards), then removes encrypted tokens,
the imported Outlook cache and Outlook availability. Appointments the person
made themselves stay in Outlook. Microsoft has no API to revoke one app's
refresh token: HiringPlug erases it; to withdraw consent entirely, remove the
app at myaccount.microsoft.com > Apps (or an admin in Entra). An identity-only
connection tombstone preserves remote event links for a same-account reconnect.
Only the holds Outlook confirmed deleted lose their link (and get a new
transaction ID, which Outlook would otherwise treat as already made); one it
refused keeps its link, so reconnecting the same account updates it instead of
making a copy. Switching accounts takes the interview and booking holds out of
the old account only after the new connection is saved, so a switch that fails
(that calendar is a colleague's) leaves the old Outlook as it was, and makes
them in the new one; it blocks automatic rerouting of existing
attendee invitations (review and explicitly retry those from the new account).
One Outlook calendar can belong to only one HiringPlug user per agency.

## Implementation and privacy

`calendars` owns its tables, Graph client, encrypted OAuth flow and core jobs.
`internal/app/calendar_adapters.go` supplies app registration, active-user checks
and projections from interviews/scheduling; modules never import each other.
Source changes enqueue `calendar.source_changed` in the source transaction.
Provider reads/writes use `calendar.refresh` and `calendar.push`, which run as
outside-transaction jobs (`jobqueue.OutsideHandlers`): no queue transaction is
held open while Microsoft answers, each step commits in its own short one, and
at most four background ones hold a database connection at once (what a person
is waiting for, such as Disconnect, never queues behind them). A job reads data
the reader may not see only through `database.AsSystem`, inside the same
transaction, so it never waits on its own row locks.

All personal event data and credentials are protected by tenant and user RLS,
even from agency owners. Schedulers read only separate intervals/health rows,
never event titles, notes, attendees or tokens. Personal endpoints refuse API
keys. Audit records contain event IDs/actions, not appointment contents.

OAuth uses signed expiring state, PKCE, authenticated one-use completion and
encrypted codes/tokens. Refresh-token rotation and provider receipts commit
independently of queue completion. A per-user advisory lock serializes provider
operations, disconnect and reconnect. An interview's own lock is held while
its invitation is checked, never while Outlook or the mail server answers, so
an edit to the interview never waits on Microsoft; an edit made meanwhile
queues another push. Graph create requests have a stable
transaction ID; queued writes bind the connection they were scheduled for. A
create Outlook stored but whose answer was lost is matched by that ID at the
next sync: the event is taken back from the import, and the appointment pushed
as it is now (deleted if cancelled meanwhile). Changing a Teams meeting never
replaces its body (the join details); other appointments send changed notes.
Untrusted continuations/redirects are rejected. Snapshots replace cached busy
times only after every page is verified; missing managed entries are looked up
by ID to distinguish a move outside the window from deletion. Temporary write
failures remain pending for retry; conflicts require review.

Tests use synthetic Microsoft HTTP servers and the real restricted PostgreSQL
role. They cover consent ownership/replay, privacy, reconnect identity, token
rotation, provider retries/conflicts, incomplete snapshots, source holds and
booking/interview changes. Live Microsoft consent, delivery and Teams behavior
still need validation against the deployment's Entra app and mailbox.
