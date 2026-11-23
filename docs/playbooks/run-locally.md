# Run it on your own machine or server

## With Docker (everything)

Double-click `update-hiringplug.command` (macOS) or `update-hiringplug.bat`
(Windows) in the project folder. It starts Docker Desktop if needed, pulls the
latest code (when `git` is installed; otherwise pull in GitHub Desktop first),
rebuilds what changed, and opens the web app. It runs:

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

Every service has `restart: unless-stopped`, so after that the app comes back
by itself whenever Docker Desktop starts (turn on "Start Docker Desktop when
you sign in" in its settings). Run the script again only after updating.

- Web app: http://localhost:3000
- API docs: http://localhost:8080/docs
- The `bootstrap` service applies migrations and creates the database roles
  each time it starts.
- Email from the app goes to **Mailpit**, which catches it (nothing reaches
  real inboxes): read it at http://localhost:8025. To send for real, set
  `HP_SMTP_HOST` (host:port), `HP_SMTP_USERNAME`, `HP_SMTP_PASSWORD`,
  `HP_SMTP_FROM` and `HP_SMTP_TLS` (`starttls` for port 587, `tls` for 465)
  before starting, e.g. in a `.env` file next to `deploy/docker-compose.yml`.

First company and owner:

```bash
docker compose -f deploy/docker-compose.yml run --rm api /app/hp tenant -name "Your Agency" -owner-email you@agency.com -owner-name "Your Name"
```

It prints a link: open it, choose your password, and you are signed in. Then
invite your team from the **Team** page (each invitation is emailed, and its
link is shown too, in case email is not set up yet). If someone cannot get an
email, make them a link on the command line:

```bash
docker compose -f deploy/docker-compose.yml run --rm api /app/hp password-link -user usr_...
```

For development, `task db:setup` (`hp dev-setup`) creates demo users who sign
in with their email (e.g. `owner@demo.hiringplug.local`) and the password
`demo-password-2026`.

**On a rented server** (your agency online, with HTTPS, off-site backups and
alerts), follow [deploy-to-a-server.md](deploy-to-a-server.md) instead.

**Before anything beyond your own laptop** (an office server, the cloud): the
passwords and secrets in `deploy/docker-compose.yml` are published defaults.
Put your own in `deploy/.env` (next to `docker-compose.yml`; never commit it):

```
HP_ENV=production
HP_PUBLIC_URL=http://<this server's address on the office network>:3000
HP_AUTH_JWT_SECRET=<32+ random characters>
HP_FILES_URL_SECRET=<32+ random characters>
HP_MAIL_KEYS_SECRET=<32+ random characters>
HP_AI_KEYS_SECRET=<32+ random characters>
HP_WEBHOOK_KEYS_SECRET=<32+ random characters>
HP_DB_ADMIN_PASSWORD=<letters and digits>
HP_DB_API_PASSWORD=<letters and digits>
HP_DB_AI_PASSWORD=<letters and digits>
```

With `HP_ENV=production` HiringPlug refuses to start while any of them is
still a published default. `HP_PUBLIC_URL` is the address others type to open
the app (links in emails use it), e.g. `http://192.168.1.10:3000`: on an
office network plain http is accepted; an address on the internet must be
https, with a real mail server (see deploy-to-a-server.md). Set the database passwords before the first start
(the admin password is fixed when the database is first created; if you set
HP_DB_ADMIN_PASSWORD later, HiringPlug stops with a message saying so: change
it inside the database first, with
`docker compose -f deploy/docker-compose.yml exec postgres psql -U postgres -c "ALTER USER postgres PASSWORD '<new>'"`). The
database, the API port and the mail catcher listen on the machine itself only;
other computers reach the web app (port 3000). Put HTTPS in front (e.g. Caddy).

## Turning on the AI assistant and matching

Both are off until an AI provider is set up. The simplest way: sign in as an
owner or admin, open **Settings > AI provider**, paste a DeepSeek, Claude or
ChatGPT API key and pick it as the one in use. That needs `HP_AI_KEYS_SECRET`
(at least 32 characters, the same for the API and the AI worker; Docker sets a
development one: set a real one for production, and keep it, or saved keys
must be entered again).

Or give the whole server one provider of its own. With Docker, put these in
`deploy/.env` (next to `docker-compose.yml`; never commit it); for development,
in your environment. They are read by the API and the AI worker. For
DeepSeek's API or any OpenAI-compatible host:

```
HP_AI_PROVIDER=openai_compatible
HP_AI_PROVIDER_URL=https://api.deepseek.com/v1
HP_AI_PROVIDER_KEY=sk-...
HP_AI_MODEL=deepseek-chat
HP_AI_MONTHLY_TOKENS=2000000
```

Then run the update again (`docker compose -f deploy/docker-compose.yml up -d`). Choose the host with your data's location in mind (PDPA):
DeepSeek's own API stores data in China; a host running DeepSeek's open model
in a region you pick, or your own server, keeps it where you choose.

## Backups

With Docker, a backup of the database and the CVs is made every night into
the `backups` folder next to this repository; the newest 14 are kept
(`HP_BACKUP_KEEP`). **Settings > System** shows the last one, and
**Settings > Backups** (the owner of the agency that runs the server) turns
the nightly backup off or on, makes one now, shows each backup's size, and
downloads one as a single .tar file to a folder you choose (Chrome and Edge
ask where; other browsers save to their downloads folder). Only that agency
sees them, because a backup holds every agency's data: set `HP_BACKUP_TENANT`
to its id once the server holds more than one agency. In Docker the backup
service runs as root and the API as another user, so the backup service hands
the backups to the API's user (`HP_BACKUP_OWNER`, set in the compose file);
they stay readable by that user only. Copy backups to another
disk or cloud storage too: a backup on the same disk does not survive the
disk. To make one now from the command line:

```bash
docker compose -f deploy/docker-compose.yml run --rm backup /app/hp backup
```

Allow enough free space on the uploads volume for a full copy of the backup's
files. Restore stages the files and checks their count before installing the
database in a transaction. After an interrupted process, keep services stopped
and preserve the `.hp-restore` directory on the uploads volume. Its
`previous` directory contains the original entries moved so far.

To put one back (it replaces every company's data and files):

```bash
docker compose -f deploy/docker-compose.yml stop api worker ai-worker
```

```bash
docker compose -f deploy/docker-compose.yml run --rm backup /app/hp restore hiringplug-YYYYMMDD-HHMMSS -yes
```

```bash
docker compose -f deploy/docker-compose.yml up -d
```

Test a restore now and then, on another machine. Without Docker, `hp backup`
needs the PostgreSQL client tools of the database's own version (it refuses
newer ones: their backup could not be put back).

## Browser smoke tests

With the API running (`task dev`) and the demo company (`hp dev-setup > devsetup.txt`):

```bash
cd web && pnpm build && HP_E2E_TOKENS_FILE=../devsetup.txt pnpm e2e
```

## Upgrading

Pull the new code and double-click the update script again (or run the `up`
command). Data is kept. Migrations run automatically, and the API refuses to
start if the database is behind.

## For development

See the quick start in the [README](../../README.md): `task db:up`,
`task db:setup`, `task dev`.

Search by meaning needs two things the compose file and `task setup` give:
PostgreSQL with the pgvector extension (the `pgvector/pgvector:pg16` image; on
a PostgreSQL installed by hand, `apt-get install postgresql-16-pgvector`), and
the meaning model in the AI worker (`task setup` downloads it to
`ai/.models/`; `HP_AI_MEANING_MODEL_DIR` points elsewhere). Without the model
the app works and says that search by meaning is not set up.
