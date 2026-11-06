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

**Before anything beyond your own laptop:** change every `*_dev` password and
the two secrets in `deploy/docker-compose.yml` (or move them to a `.env` file),
and put HTTPS in front (e.g. Caddy).

## Turning on the AI assistant and matching

Both are off until an AI provider is set up. The simplest way: sign in as an
owner or admin, open **Admin > AI provider**, paste a DeepSeek, Claude or
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

```bash
docker compose -f deploy/docker-compose.yml exec postgres pg_dump -U postgres hiringplug > backup.sql
```

Also back up the `files` volume (the CVs). Test a restore regularly.

## Upgrading

Pull the new code and double-click the update script again (or run the `up`
command). Data is kept. Migrations run automatically, and the API refuses to
start if the database is behind.

## For development

See the quick start in the [README](../../README.md): `task db:up`,
`task db:setup`, `task dev`.
