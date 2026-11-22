# Apollo Cloudflare Worker

This Worker is based on the broker supplied in the reviewed CV Studio
handover. HiringPlug uses protocol 2 for work-email lookup and revocation.
The existing CV Studio routes and D1 schema remain compatible.

For an existing approved broker, deploy `worker.mjs` with Wrangler to the same Worker
and retain its D1 binding, secrets, public origin, and registered callback.
Validate the updated Worker in a staging deployment first if the broker is
shared with CV Studio. The public origin ends in `.workers.dev`.

For a new deployment, use a D1 binding named `DB` initialized with
[`schema.sql`](schema.sql). Set `PUBLIC_ORIGIN` and the Worker secrets
`APOLLO_CLIENT_ID`, `APOLLO_CLIENT_SECRET`, and `TOKEN_KEY`. The token key
must contain 64 random hexadecimal digits. Register `PUBLIC_ORIGIN/callback`
with the approved Apollo OAuth app and keep the key when deploying updates.
Schedule the Worker's cleanup handler for expired flows, accounts, and
lookup records. HiringPlug's server settings are in the
[setup guide](../../docs/apollo-hr.md).

Run the broker tests with Node 22.13 or newer (they use the built-in
`node:sqlite`, which older Node 22 releases keep behind
`--experimental-sqlite`).

```sh
node --test tools/apollo-broker/worker.test.mjs
```

The tests use SQLite and fictional provider responses. A live deployment
check should cover sign-in and revocation before anyone confirms a paid lookup.
