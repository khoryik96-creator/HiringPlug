# Outlook calendar

Calendar (under More in the top bar) is each person's private calendar. It has
month, week and agenda views, local appointments, Outlook events and the
person's HiringPlug interviews and booked calls. Email connections continue to
work independently.

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
  The existing candidate ICS/email invitation flow remains. A connected booking
  host receives the native hold instead of another ICS email copy.
- An Outlook change to an own appointment is reflected on the next refresh.
  A changed/deleted interview or booking hold is flagged for review; its source
  schedule stays in HiringPlug. **Keep HiringPlug details & sync** explicitly
  resolves a conflict. Changes should otherwise be made in the original interview
  or booking link. Concurrent edits use versions and Outlook conditional writes.
- Booking pages and interview clash checks use Outlook busy intervals. A failed,
  pending or older-than-15-minute snapshot is unknown availability: scheduling
  reports an error rather than offering potentially occupied times. Existing
  interview ignore-clashes behavior remains an explicit override. The optional
  ICS feed remains available; native Outlook takes precedence while connected.

Disconnect removes encrypted tokens, the imported Outlook cache and Outlook
availability. It does not delete existing Outlook events. An identity-only
connection tombstone preserves remote event links for a same-account reconnect.
Switching accounts blocks automatic rerouting of existing attendee invitations;
review and explicitly retry such appointments from the new account. One Outlook
calendar can belong to only one HiringPlug user per agency.

## Implementation and privacy

`calendars` owns its tables, Graph client, encrypted OAuth flow and core jobs.
`internal/app/calendar_adapters.go` supplies app registration, active-user checks
and projections from interviews/scheduling; modules never import each other.
Source changes enqueue `calendar.source_changed` in the source transaction.
Provider reads/writes use `calendar.refresh` and `calendar.push`.

All personal event data and credentials are protected by tenant and user RLS,
even from agency owners. Schedulers read only separate intervals/health rows,
never event titles, notes, attendees or tokens. Personal endpoints refuse API
keys. Audit records contain event IDs/actions, not appointment contents.

OAuth uses signed expiring state, PKCE, authenticated one-use completion and
encrypted codes/tokens. Refresh-token rotation and provider receipts commit
independently of queue completion. A per-user advisory lock serializes provider
operations, disconnect and reconnect. Graph create requests have a stable
transaction ID; queued writes bind the connection they were scheduled for.
Untrusted continuations/redirects are rejected. Snapshots replace cached busy
times only after every page is verified; missing managed entries are looked up
by ID to distinguish a move outside the window from deletion. Temporary write
failures remain pending for retry; conflicts require review.

Tests use synthetic Microsoft HTTP servers and the real restricted PostgreSQL
role. They cover consent ownership/replay, privacy, reconnect identity, token
rotation, provider retries/conflicts, incomplete snapshots, source holds and
booking/interview changes. Live Microsoft consent, delivery and Teams behavior
still need validation against the deployment's Entra app and mailbox.
