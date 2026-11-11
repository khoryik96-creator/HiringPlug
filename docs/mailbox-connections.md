# Connecting email

Settings > Email connection belongs to the signed-in user in the current agency.
Microsoft, Google and authenticated SMTP are supported independently of the
user's login domain or the recipients' domains. One active mailbox is selected
per user per agency. This is outbound email; it does not synchronize an inbox.

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
claim. A later disconnect, new authorization or SMTP replacement invalidates an
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
