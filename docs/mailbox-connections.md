# Connecting email

Settings > Email connection belongs to the signed-in user in the current agency.
Microsoft, Google and authenticated SMTP are supported independently of the
user's login domain or the recipients' domains. One active mailbox is selected
per user per agency. Microsoft also supports the opt-in Outlook Mail workspace described below.

Existing server SMTP remains the fallback for users without a connection and for
system-generated mail (password resets, calendar notices and invoices). Configure
`HP_SMTP_*` with a real sending service for production system mail. Docker's
`mailpit:1025` is a local catcher; it does not deliver to outside inboxes. The
composer warns about this and new sent records say "Captured in test inbox".
"Accepted by mail server" means provider acceptance, not proof of inbox arrival.
Historical records without a recorded transport cannot establish live delivery.

## Deployment setup

Set the same stable `HP_MAIL_KEYS_SECRET` on the API and Go worker. Generate at
least 32 random characters and keep it in deployment secret storage. Never put
real secrets in a commit or support message. Development/test has a development
fallback; production leaves connections disabled when this variable is empty.
Changing this key makes existing connections unreadable: reconnect those
mailboxes. Restart/recreate both API and Go worker after changing environment
variables. Set `HP_PUBLIC_URL` to the browser-visible application origin.

**The Microsoft and Google app details can be entered in Settings > Email
connection** (owners and admins: `emails.apps:manage`) instead of the
`HP_MAIL_MICROSOFT_*` and `HP_MAIL_GOOGLE_*` settings below: the client ID,
and the client secret, which is stored sealed with `HP_MAIL_KEYS_SECRET` and
never shown again (only its last four characters). Each agency's own app is
used to connect and refresh its people's mailboxes; without one the server's
settings are used. The page also shows the redirect URI to register. Changing
or removing an app means mailboxes connected through the old one connect again.
`HP_MAIL_KEYS_SECRET` itself stays a server setting: it is the key that locks
these secrets, so it cannot be kept in the database beside them.

The OAuth redirect registered with each provider must exactly equal:

```
<HP_PUBLIC_URL>/v1/mailbox/callback
```

For the default local Docker setup that is
`http://localhost:3000/v1/mailbox/callback`. Production uses HTTPS. Redirects
return to `/#/email-connection` with only a success/failure indicator; tokens,
provider errors and authorization codes never enter the frontend. Callback
responses are not cached or used as referrers, and the bundled nginx proxy does
not access-log the callback query.

### Microsoft

Register a web application in Microsoft Entra. For a shared SaaS deployment,
choose organizational directories plus personal Microsoft accounts if both
Microsoft 365 and Outlook.com should be supported. Register the redirect above,
create a client credential, and configure:

```
HP_MAIL_MICROSOFT_CLIENT_ID=<application-client-id>
HP_MAIL_MICROSOFT_CLIENT_SECRET=<client-secret>
```

Delegated Graph permissions: `User.Read`, `Mail.Send`, `Mail.ReadWrite`, plus
`openid` and `offline_access`. Mail.ReadWrite is used to create and clean up
outgoing drafts for large CV attachments. Ordinary small messages use MIME
sendMail; larger messages use a draft and chunked attachment upload so the ATS's
20 MB attachment allowance is retained. Tenant consent policy may require an
administrator to approve the application. Configure the application for the
intended account audience; a single-tenant registration limits who can sign in.

### Google

Register a Google OAuth web client with the redirect above, configure its
consent screen, and enable the Gmail API. Configure:

```
HP_MAIL_GOOGLE_CLIENT_ID=<oauth-client-id>
HP_MAIL_GOOGLE_CLIENT_SECRET=<client-secret>
```

Scopes: `openid`, `email`, `https://www.googleapis.com/auth/gmail.send`.
The account email must be provider-verified. Refresh access uses the provider's
offline consent. A Google application in testing is restricted to its configured
test users and may receive short-lived grants. Serving other agencies requires
the appropriate published/verified consent configuration, subject to Google's
policies and Workspace administrator controls.

Neither provider button is enabled until its application credentials and secure
storage are configured. Real provider sign-in and external receipt still need
to be checked using the deployed registrations; local provider stand-ins verify
the protocol and delivery paths but do not prove production provider consent.

### Other providers / SMTP

Choose Other provider (SMTP) and enter the sender address, server, port, username
and password/app password supplied by the provider. Use STARTTLS (usually 587)
or implicit TLS (usually 465). Use Microsoft/Google sign-in for those providers
rather than assuming ordinary password authentication will be accepted.
Passwords are write-only. Leaving one blank retains it only if the server,
port, security, username and sender address are unchanged. Some IP-authorized
relays accept an empty username/password. Saving SMTP stores the configuration;
the first send establishes actual server acceptance or records a useful failure.

User-supplied SMTP servers cannot connect to private/local networks in production;
checks apply to the actual resolved socket address. Plaintext SMTP is allowed
only when the existing development-only private-network option is enabled.
TLS certificate verification remains enabled. Remote error text is not logged
or stored, because an SMTP response could echo credentials or message content.

## Ownership and delivery semantics

API keys cannot manage personal mailbox connections. The emails module owns both mailbox and OAuth-state tables. Every record is
scoped by tenant and user, with row-level security and user foreign keys. Stored
passwords and tokens use AES-GCM with tenant/user/provider-bound authenticated
data. API responses expose only connection metadata; audit entries contain the
provider and action, never passwords, tokens or message contents.

Authorization uses an expiring signed state, PKCE S256 and a one-use database
claim. The provider's redirect (`/v1/mailbox/callback`) connects nothing: it
parks the sign-in code, sealed, on the state and sends the browser back to
Settings > Email connection with a one-time ticket. The person who started the
connection finishes it there, signed in (`POST /v1/mailbox/complete`); the
state is theirs alone (row-level security), so a sign-in link passed to someone
else cannot connect that person's mailbox to another account. A later disconnect, new authorization or SMTP replacement invalidates an
in-flight callback before it can save credentials. An advisory lock serializes
connection changes for one tenant/user, including initial concurrent creates.

Each queued message records its selected mailbox identity and test/live mode.
Disconnecting/replacing the mailbox fails pending mail for that identity; it
never silently switches sender or falls back to a local catcher. A message
already being transmitted may finish. Server fallback messages are not moved
between test and live modes after queuing. Review and send again when a route
has changed; failed/old emails are not automatically resent.

Refresh tokens can rotate before a later send fails. A refresh therefore commits
in a separate, short transaction and temporary connection, under a per-mailbox
row lock, before delivery. It survives a send rollback and avoids waiting for a
spare worker-pool connection while every worker has an open job transaction.
Most sends use the existing valid access token and open no extra connection.
Provider acceptance and the email/job state retain the queue's existing
at-least-once semantics; a timeout or commit failure can cause a duplicate retry.

Google and small Microsoft messages send MIME with a provider-only Bcc header;
the provider derives the hidden recipients and removes that header on delivery.
The SMTP transport uses its envelope instead. HTML/plain text, inline banners
and attachments are preserved. Microsoft's preauthorized attachment upload URL
is checked against its upload hosts and receives no OAuth bearer token. Local
HTTP provider overrides exist only for in-process tests, never as user input.

Calendar uses the same Microsoft app registration with a separate consent flow
and `/v1/calendar/callback` Web redirect. See [calendar.md](calendar.md) before
adding `Calendars.ReadWrite`; existing mailbox tokens/scopes remain separate.

## Outlook Mail

Open the Outlook icon beside Calendar after connecting Microsoft. Choose **Enable sync**
to index your personal Outlook mailbox in this agency. Outbound email keeps working
through the same connection. Outlook Mail uses the existing delegated Microsoft
permissions. Shared or delegated mailboxes require a separate workflow.
Hover over an action icon or focus it with Tab to see its label.

HiringPlug discovers visible folders and reads message delta pages in the Go
worker. Initial indexing covers up to 90 days of mail, subject to Microsoft's
5,000-result limit for filtered delta queries per folder. Drafts are indexed
without the date filter. Large mailboxes fill in over several background runs.
Open a message to read it in its own formatting (fonts, colours, tables, and
its pictures: signature logos and banners, whether sent with it or linked from
the web) and to see its attachments and download files up to 20 MiB. The
message is made safe before it is shown (nothing in it can run) and is shown in
a frame of its own. Pictures from the web load as in Outlook, over https and
without telling their site which page asked; in **Junk Email** they wait for
**Show pictures**, so a sender of junk cannot tell the message was opened. Open the message in Outlook for linked items, larger files,
additional attachments or full history. Message details and files are fetched
when requested. HiringPlug indexes subject, the sender's name, participants,
dates, flag and folder/read state locally and does not store bodies or
attachment contents in the inbox index.

The page is laid out like Outlook: folders with their unread counts (Deleted
Items and Junk Email included, sub-folders under their parent), then the
message list (sender, subject and attachment mark; beside each date, its
**Flag** and **Move to**), then the reading pane. The command bar above them
holds **New mail**, then **Delete** (to Deleted Items), **Archive** and **Mark
as read/unread** as icons named in tooltips; **Reply**, **Reply all** and
**Forward** sit as icons beside the sender. Opening a message marks it read, as Outlook
does.

For a CV attachment, choose **Save CV**, select the right candidate and confirm
**Save to CV & files**. **Save CV** is offered for files within the upload limit
for candidate files (`HP_MAX_UPLOAD_BYTES`); larger ones can still be
downloaded. Opened from a candidate's page, the candidate is suggested only
for their linked mail, never for other messages read from there. HiringPlug checks file types and skips exact duplicates
using the same upload rules as the candidate profile. People who can access the
candidate's CVs can see the saved copy. The Outlook message stays private.

Choose **Import CV** to open an attachment within the upload limit in **Import profiles**. Choose
the candidate source and confirm the import to start reading the CV. The
normal import checks for existing copies and candidates with the same email.
A file whose upload fails stays selected so you can retry it.

Read/unread, move and archive update the original Outlook message. Outlook changes
arrive through polling, which also works on a laptop without a public webhook
address. **Reply** and **Reply all** open a rich Outlook draft with the
original conversation and Outlook-chosen recipients. Add your response, then
save, send, or schedule it. While the Drafts folder first syncs, you can still
send a quick reply from the reading pane.
Choose **Forward**, enter the recipients and an optional note, then select
**Review forward**. Outlook copies the original message and attachments into a
draft. Review, add or remove files before you send or schedule the draft.

On a candidate or contact's **Email** tab, open **My Outlook conversations**.
**Find in Outlook** starts an exact address search. Open a message and confirm the
profile link. Linking a conversation does not share it with other recruiters or
admins. It remains visible only to you and only through profiles you can access.
**Turn off sync** or disconnecting/replacing the mailbox removes its local index
and links. Your original mail and drafts remain in Outlook.

### Inbox rules

Open **Inbox rules** from Outlook Mail. The first time, choose **Allow inbox
rules** and sign in to the same connected Outlook mailbox. In Entra, the app
registration needs Microsoft Graph delegated **MailboxSettings.ReadWrite**.
Have an administrator grant consent if your organisation requires it. The
existing email and calendar permissions still use their own consent.

Name the rule, pick a synced folder by its full folder path, and enter a sender email address, text
to find in the subject, or both. When you enter both, both must match. Outlook
adds the rule after your existing rules and applies it to incoming messages.
It can do this while HiringPlug is closed. Turn rules on or off, or delete
them, in HiringPlug. Read-only rules and advanced editing are handled in Outlook.

### Drafts and scheduled email

Use **New message** to start an Outlook draft, or open one from **Drafts**.
Save edits to the same draft, discard it to Deleted Items, send it now, or
choose **Send later**. The editor supports bold, italic, underline, lists,
links, alignment, indentation, fonts, sizes and text colour. Opening a draft
keeps its original formatting until you edit the message. Use **Attach file** to add a file
up to 10 MB. Use the trash icon beside a file to remove it from the draft.
Adding a file to a new message first saves it to Outlook Drafts. If you change
attachments on a scheduled message, review the files and update the schedule.
Use Outlook to edit inline images or a message that is too long to edit in
HiringPlug.

**Write with HiringPlug** opens the ATS email composer within Outlook Mail.
Use it for templates, signatures, banners, candidate details, CV selection,
previews, and AI writing help when enabled. When opened from a candidate or
contact's Outlook conversations, the composer uses that profile's context.
These emails follow HiringPlug's usual delivery and ATS history rules.
**Send later** is available from native Outlook drafts.

The **Scheduled** list shows the send time in your time zone. Open the draft
to change its content or time, or choose **Cancel send** to leave it in
Outlook Drafts. **Update schedule** applies the edits and the new send time
together. A pending send stays tied to the same mailbox and draft even if the
mail index is rebuilt. If Outlook changes the draft, reopen it and review it
before updating the same schedule. Turning off Outlook sync
or disconnecting or replacing the mailbox cancels its pending sends.
Allowing inbox rules for the same connected mailbox keeps its scheduled sends.
Reauthorizing the same Outlook mailbox also keeps drafts, schedules and the
mail index. Use **Next** or **Previous** to see more sends in the Scheduled list.

The HiringPlug Go worker must be running to send at the chosen time.
If the worker comes back within 24 hours of the send time, it sends the
reviewed draft after rechecking access and consent for the To, Cc, and Bcc
addresses, including unlinked recipients. Later sends need review.
HiringPlug submits the Outlook draft once and checks the same immutable ID
for its sent copy. If Outlook's response is lost, HiringPlug checks the
receipt without resubmitting. **Check Outlook** means the outcome could not
be confirmed. **Check Outlook again** checks the original message. If Outlook
still has the draft, you can review it and verify Sent Items before scheduling
it again. Rechecking never sends email.
