# Put HyPlug on a server (your agency online)

This puts HyPlug on a rented server so your team opens it in the browser
from anywhere, at your own web address, with the padlock (HTTPS). It takes an
afternoon. Your part is creating accounts; the commands below can be copied
as they are.

What you get:

- **HTTPS:** the certificate is obtained and renewed by itself.
- **Live mode:** the app refuses to start with a practice password, the test
  mail catcher, or an address that is not `https://`.
- **Backups:** every night, kept on the server (14 nights) and copied to
  storage somewhere else.
- **Alerts:** an email when something needs someone (a failed backup, a
  nearly full disk, background work failing), and an outside monitor that
  tells you if the server is down.

## 1. What you need first

| What | Example | Rough cost |
|---|---|---|
| A server: Ubuntu 24.04, 4 CPUs, 8 GB memory, 160 GB disk or more (200,000 CVs take 50–150 GB, and search by meaning about 15 GB more: 60–90 KB a candidate, not in backups) | Hetzner, DigitalOcean, Vultr, AWS Lightsail; Singapore or Malaysia region (PDPA) | RM 250–500 / month |
| A web address | `app.youragency.com` | about RM 50 / year |
| Email sending (SMTP) | Google Workspace, Microsoft 365, Amazon SES, Brevo, Mailgun | often included, or RM 0–60 / month |
| Storage for the off-site backups (S3-compatible) | Wasabi, Backblaze B2, DigitalOcean Spaces, AWS S3 | RM 20–60 / month |
| An uptime monitor | UptimeRobot or Better Stack (free plans) | free |

## 2. Point your web address at the server

In your domain's DNS settings, add an **A record**: name `app` (or whatever
you chose), value the server's IP address (from the provider's page). It can
take up to an hour to work. HTTPS cannot start before it does.

## 3. Install Docker and get HyPlug

Sign in to the server (the provider shows how: `ssh root@<server-ip>`), then:

```bash
apt-get update && apt-get install -y git ufw
```

```bash
curl -fsSL https://get.docker.com | sh
```

```bash
ufw allow OpenSSH && ufw allow 80 && ufw allow 443 && ufw --force enable
```

```bash
git clone https://github.com/<your-account>/hiringplug.git /opt/hiringplug && cd /opt/hiringplug
```

(For a private repository GitHub asks for a password: use a fine-grained
personal access token with read access to this repository only.)

## 4. Your settings: `deploy/.env`

Create the file and fill it in (`nano deploy/.env`). Never commit it or send
it to anyone. Make each secret with `openssl rand -hex 32` (a new one each
time) and each password with `openssl rand -hex 16`.

```ini
# Where people reach the app
HP_DOMAIN=app.youragency.com

# Secrets and passwords (each different; set the database ones before the first start)
HP_AUTH_JWT_SECRET=
HP_FILES_URL_SECRET=
HP_MAIL_KEYS_SECRET=
HP_AI_KEYS_SECRET=
HP_WEBHOOK_KEYS_SECRET=
HP_DB_ADMIN_PASSWORD=
HP_DB_API_PASSWORD=
HP_DB_AI_PASSWORD=

# Email from the app (your provider's SMTP settings; port 587 = starttls, 465 = tls)
HP_SMTP_HOST=smtp.yourprovider.com:587
HP_SMTP_USERNAME=
HP_SMTP_PASSWORD=
HP_SMTP_FROM=no-reply@youragency.com
HP_SMTP_TLS=starttls

# Who is emailed when the server needs someone (several: separate with commas)
HP_ALERT_EMAIL=you@youragency.com

# Off-site backups: your bucket and a key that may write to it only
HP_OFFSITE_S3_BUCKET=youragency-hiringplug-backups
HP_OFFSITE_S3_ENDPOINT=s3.ap-southeast-1.wasabisys.com
HP_OFFSITE_S3_REGION=ap-southeast-1
HP_OFFSITE_S3_ACCESS_KEY=
HP_OFFSITE_S3_SECRET_KEY=

# Filled in at step 6
HP_BACKUP_TENANT=
```

Notes:

- **Email:** ask your provider for its SMTP settings. Also add the SPF and
  DKIM records it gives you to your domain's DNS, or emails land in spam.
- **Off-site storage:** make a private bucket in the provider's region
  nearest you, and a key limited to that bucket. For AWS itself, leave
  `HP_OFFSITE_S3_ENDPOINT` empty.
- **AI (optional):** see `HP_AI_PROVIDER` in `.env.example`.
- Every other setting is in `.env.example` with what it does.

## 5. Start it

```bash
sh deploy/update-server.sh
```

The first build takes 5–10 minutes. It ends with "HyPlug is running at
https://app.youragency.com". If a setting is missing or still a practice one,
the app says which: read it with
`docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml logs api`.

## 6. Create your agency and your own sign-in

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml run --rm bootstrap /app/hp tenant -name "Your Agency" -owner-email you@youragency.com -owner-name "Your Name"
```

It prints the agency's id (`tnt_...`) and a link to choose your password.
Open the link, choose a password, and sign in. Then put the id in
`deploy/.env` as `HP_BACKUP_TENANT=tnt_...` (so you can manage backups in
Settings › Backups) and run `sh deploy/update-server.sh` again. Invite your
team from Settings › Team.

## 7. The uptime monitor

In UptimeRobot (or Better Stack), add an HTTP monitor for
`https://app.youragency.com/health/ready`, every 5 minutes, alerting your
email (and phone, if you like). It answers while the app and its database
work; anything else means the server needs someone.

## 8. Check the backups, once (the restore drill)

The day after starting, Settings › System should say "Also copied off the
server". Then prove the copy comes back, on a **test** server (never the live
one): do steps 3–5 there with the same off-site settings, then:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml run --rm backup /app/hp fetch
```

Check the copy reads back before stopping anything (every backup is checked
as it is made; this checks the one brought back):

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml run --rm backup /app/hp backup verify hiringplug-YYYYMMDD-HHMMSS
```

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml stop api worker ai-worker
```

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml run --rm backup /app/hp restore -yes hiringplug-YYYYMMDD-HHMMSS
```

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml up -d
```

(`hp fetch` prints the backup's name for the restore; `-yes` may also come
after it.) The restore exits non-zero if anything fails, and prints what it
did:

- **Files**: they are given to the user the API and workers run as
  (`HP_FILES_OWNER`, 65532:65532 in compose), still private to it (folders
  0700, files 0600), before they replace the current ones: if that fails,
  nothing is replaced. Once the database and files are back, a sample is
  opened as that user; if that fails the restore exits non-zero and prints
  the command to run: `... run --rm backup /app/hp repair-files -apply`, with
  the API and workers still stopped (it refuses while they run, and never
  follows a link). The kept backups are never changed: the restore copies
  their files. A server restored before 1.47.0 whose CV previews say
  "permission denied" is fixed the same way (without `-apply` it only
  reports); Word CVs whose PDF copy failed meanwhile get theirs from the
  file's menu, **Make the PDF again**.
- **Jobs**: the database is brought up to date, and the jobs waiting in the
  backup that reach people or other systems (emails, interview reminders,
  digests, invoices, automations, calendar updates, webhooks, claim uploads,
  Apollo and mailbox checks, and the Outlook, calendar, job-board and
  company-website reads that use the restored sign-ins) are **held**: they
  may have happened already, and a test server must never email real
  candidates. Paid AI work waiting in the backup (fit scores, rankings, CV
  formatting, AI readings...) is held too, so nothing is billed twice.
  Settings › System lists them in each company, to release or cancel, each
  or all; the agency that runs the server (`HP_BACKUP_TENANT`) sees every
  company's. On the server, `... run --rm backup /app/hp jobs list` lists
  every company's, and `hp jobs release -yes` or `hp jobs cancel -yes` (add a
  job's id for one) settles them. After a restore from another server the
  alert email waits until none is held, 72 hours at most. Work that stays
  inside HyPlug (reading CVs, PDF copies, search by meaning) runs again
  by itself. `hp restore -yes -resume-outbound NAME` lets the outbound kinds
  run at once, and `-resume-paid-ai` the paid AI work (and leaves paid AI
  on): only when you know they did not happen.
- **Paid AI**: a backup from another server (the restore prints both) is
  put back with paid AI paused for every company, company keys included, so
  a copy spends nobody's credit. The agency that runs the server
  (`HP_BACKUP_TENANT`, or the only one) turns it back on in Settings ›
  System, or anyone on the server with `... run --rm backup /app/hp paid-ai
  on` (`HP_AI_PAID=off` in deploy/.env keeps it off whatever they say). CVs
  are still read, by the rules.

Sign in on the test server and check a few candidates and their CVs (open a
PDF, a Word CV's preview, upload a new one), then delete the test server.
If the live server is ever lost, the same four commands on a new server bring
everything back; then release the held jobs that still need doing and turn
paid AI back on.

Sharing the storage is safe: each server deletes there only its own oldest
backups (as many as it keeps), never another server's, and uploaded files
that are gone from a server are set aside there, not deleted, while any backup
kept there may need them. `hp fetch` picks the newest whole backup made by
another server (on a new server, its own first backups hold only an empty
database); name one to fetch it instead (`hp fetch hiringplug-YYYYMMDD-HHMMSS`).

## 9. Keeping it up to date

```bash
cd /opt/hiringplug && sh deploy/update-server.sh
```

Updates keep your data; the database is updated by itself.

## What the alerts email covers

The worker checks every 15 minutes and emails `HP_ALERT_EMAIL` when a problem
starts, once a day while it lasts, and once when it is fixed:

- the last backup failed, or none for two days while the nightly backup is on
- the last backup was not copied off the server
- the backup service stopped checking in
- background work (emails, CV reading...) failed for good in the last day, or
  waited over an hour
- the disk has less than a tenth (or 2 GB) free
- the AI worker (or the background worker) stopped checking in, or still runs
  the old version after an update (1.52.0): restart it
  (`docker compose ... up -d`)

Settings › System › **Server checks** shows the same, live, with the workers'
versions, the files folder and the free disk space.

A server that is down cannot email: the uptime monitor (step 7) covers that.
