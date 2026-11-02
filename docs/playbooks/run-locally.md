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

First company and owner:

```bash
docker compose -f deploy/docker-compose.yml run --rm api /app/hp tenant -name "Your Agency" -owner-email you@agency.com -owner-name "Your Name"
docker compose -f deploy/docker-compose.yml run --rm api /app/hp token -user usr_...
```

Until single sign-on arrives (M7), sign in with that token.

**Before anything beyond your own laptop:** change every `*_dev` password and
the two secrets in `deploy/docker-compose.yml` (or move them to a `.env` file),
and put HTTPS in front (e.g. Caddy).

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
