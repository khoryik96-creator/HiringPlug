# Claims storage

Owners and admins (`claims.storage:manage`) connect one company account in
**Settings > Claims storage**: Google Drive, OneDrive or Dropbox. From then on,
each expense claim's receipt is uploaded to it when the claim is submitted.

## Where receipts go

```
<top folder>/<YYYY-MM>/<person>/<receipt type>/<YYYY-MM-DD> <merchant> <currency> <amount> (<end of the claim id>).<ext>
Claims/2026-11/Aisyah Rahman/Grab - e-hailing fare/2026-11-03 Grab MYR 23.00 (t5v6w7).pdf
```

- The month and day are the receipt's date; the person is the claimant's name;
  the receipt type is its label in Settings > Dropdowns & categories.
- Characters drives refuse (`\ / : * ? " < > |`) become `-`; names are kept
  short. The end of the claim's id keeps two alike receipts apart.
- The top folder (default `Claims`, e.g. `Finance/Claims`) applies to receipts
  submitted after it is changed; files already uploaded stay where they are.
- A claim rejected and submitted again replaces its earlier file. A deleted
  claim's file is removed. Disconnecting keeps the files.

## How it works

`POST /v1/claims/submit` queues a `claim.upload` job (core queue) for each claim
in the same transaction, when an account is connected. The job runs outside a
transaction (it waits on the provider): it reads the claim, gets an access
token (refreshing it and keeping the new tokens), deletes the claim's earlier
file in the same account, uploads the receipt and records where it went. An
upload sequence on the claim makes an older job (the claim submitted again
since) do nothing.

- **Provider busy or unreachable** (timeouts, 429, 5xx): the queue tries again,
  with its usual back-off; after the last attempt the claim shows *failed*.
- **Access withdrawn** (401, or the refresh token refused): the storage is
  marked *connect again* and the claim *failed*. Connecting again (the same or
  another account) queues every unfinished upload again.
- **Other refusals** (e.g. the drive is full): the claim shows *failed* with the
  reason; **Try again** in Settings queues the failed uploads again.

Tokens are sealed with AES-GCM (keyed from `HP_MAIL_KEYS_SECRET`) and never
shown or logged. Sign-in is OAuth with PKCE: the public callback only parks the
provider's code, and the signed-in person who started it finishes it with a
one-time ticket. No salary values are involved.

## Setting up the apps

Register the redirect URL `<HP_PUBLIC_URL>/v1/claims/storage/callback` (shown on
the page) in each app you use.

| Provider | App | Permissions |
| --- | --- | --- |
| OneDrive | the Microsoft app in Settings > Email connection (or `HP_MAIL_MICROSOFT_CLIENT_ID`/`_SECRET`) | `Files.ReadWrite`, `User.Read`, `offline_access` (delegated) |
| Google Drive | the Google app in Settings > Email connection (or `HP_MAIL_GOOGLE_CLIENT_ID`/`_SECRET`); turn on the Google Drive API | `drive.file` (only the folders and files HiringPlug makes), `email` |
| Dropbox | a Dropbox app (scoped access, full Dropbox) set in `HP_DROPBOX_APP_KEY` and `HP_DROPBOX_APP_SECRET` | `account_info.read`, `files.content.write` |

With Google's `drive.file` permission HiringPlug cannot see folders people made
themselves, so it makes its own top folder (a folder of the same name made by
hand stays separate).
