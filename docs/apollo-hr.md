# Apollo work email in Find HR

Open **Settings → Integrations → Apollo** (or **Companies → Find HR → Apollo**). Choose **Connect Apollo**, open the
Apollo sign-in page, approve access with your own Apollo account, then return
to HiringPlug and choose **Check connection**.

Choose **Work email** on a Find HR result. HiringPlug first checks for a work
address on a contact with that exact LinkedIn profile at that company. If
there is no usable address, review the person and confirm the Apollo lookup.
Apollo may use a credit even without returning an email. HiringPlug shows
Apollo's verification status and leaves the contact unchanged.

If the result is unconfirmed, use **Check lookup status** to retrieve the
original result. The broker retains its lookup status for 24 hours. After
that window, or if you have since changed Apollo connections, check Apollo
for that result and any credit use.

## Deployment

Deploy the [HiringPlug Apollo Worker](../tools/apollo-broker/README.md)
before enabling this feature. It uses the CV Studio broker's D1 schema,
approved Apollo partner OAuth client, and exact HTTPS
`/callback` redirect registered in Apollo. The broker needs
`read_user_profile`, `people_match`, and `app_scopes`. The broker must verify
both the LinkedIn profile and its employer before revealing an address.
The employer name alone cannot establish a match: the employer's LinkedIn page
name must match the scanned company page, or LinkedIn's company number must
(Apollo's `linkedin_uid`, which also covers companies saved by their number,
`/company/123456/`). HiringPlug uses the Worker's
versioned lookup routes and requires identity verification on a found result.

Set both variables on the API and the Go worker. Docker Compose passes these
through from the deployment's environment file. On native installations, add
them to both service environments.

```dotenv
HP_APOLLO_BROKER_URL=https://<worker>.<account>.workers.dev
HP_APOLLO_KEYS_SECRET=<dedicated random secret of at least 32 characters>
```

Keep the encryption key across upgrades in the deployment's secret store or
private environment file. Connect each recruiter's own account from Find HR.
The Apollo application secret and refresh tokens remain on the broker. A
shared deployment can continue serving CV Studio through its original routes.
To enable Apollo on a Find HR search made before this release, run a new
search so HiringPlug can bind the company page used in that scan.

## Setup checks

- **The broker blocked the request.** Check Cloudflare's security events and
  its rules for the Worker. HiringPlug uses the user agent that worked with
  the CV Studio broker. An HTML 403 from Cloudflare means the request may
  have stopped before Apollo.
- **Apollo doesn't recognise the app or denies consent.** Check the partner
  OAuth client, approved scopes, exact registered redirect, and the account's
  Apollo plan and permissions. Then cancel the existing attempt and reconnect.
- **Apollo says sign-in succeeded, but HiringPlug is still waiting.** Click
  **Check connection** in the browser where you started. If the ten-minute
  window elapsed, sign in again. Verify the Worker origin and deployed callback
  match the HiringPlug setting if the problem repeats.
- **An address was withheld.** HiringPlug shows when Apollo returned only a
  personal address, a redacted address or a mismatched profile/employer.
  Broker errors and missing addresses are insufficient to establish credit use.

The `contacts.apollo:use` permission controls access; a lookup also needs
`contacts:read`, because it shows the Find HR scan and saved addresses. The
worker checks the recruiter's access again just before the paid call, so a
role changed after the confirmation sends nothing. Connection proofs and
returned addresses are encrypted in PostgreSQL and scoped to the recruiter
and tenant. An explicit confirmation creates a lookup record and Go worker
job together. The worker commits a claim before dispatch; duplicate clicks
and worker retries read the same record. A lookup proven to have stopped
before dispatch can be confirmed again. That includes a request that expired
in the queue and a broker limit that occurred before Apollo was called.
A limit returned by Apollo after dispatch needs a review in Apollo for any
credit use. A lookup with an uncertain result
can only be checked, including after repeat clicks. Results from an earlier
connection stay private to that connection.

If the broker cannot confirm a disconnect, HiringPlug pauses lookups and keeps
the encrypted proof so **Retry disconnect** can revoke the same connection.
If the original broker or encryption key is unavailable, revoke the app in
Apollo and then forget the connection in HiringPlug.

HiringPlug ships the Leads schema migration `00156` with Apollo so normal
deployments apply both in order. The migrator also supports databases that
ran Apollo's `00157` preview before the Leads migration was bundled.
