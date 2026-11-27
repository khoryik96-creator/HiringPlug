# Let someone try HyPlug on GitHub Codespaces

A codespace is a computer in the cloud that GitHub lends you. HyPlug runs
there exactly as on a server (the same Docker stack), so a colleague can try it
from a link and send comments. It is for trying out, not for real use: it
sleeps when idle, and its data can be deleted. **Use made-up candidates only**,
never real CVs.

## Start it (the first time takes 10-15 minutes)

1. On GitHub, open the repository, click **Code** › **Codespaces** ›
   **…** › **New with options**, pick the branch (usually `main`) and a
   **4-core** machine, then **Create codespace**.
2. Wait. The terminal shows "Starting HyPlug…" while it builds, then
   "HyPlug is running" and a link: **The owner chooses their password
   here**. Open that link (it works for 7 days) and choose your password. You
   sign in with your GitHub email. The link is also kept in
   `CODESPACE-SIGN-IN.txt`; `bash .devcontainer/password-link.sh` makes a new
   one.
3. Let your colleague in: open the **Ports** tab, right-click the
   **HyPlug (3000)** row › **Port visibility** › **Public**. Its address
   (`https://…-3000.app.github.dev`) is the app's link.
4. In HyPlug, **Settings › Team › Invite**: enter your colleague's email
   and role. Copy the invite link it shows and send it to them (the codespace
   sends no real email; the messages it would send are in the **Ports** tab
   under "Emails the app sent").
5. AI features stay off until you add a key: **Settings › AI provider**.

## While they try it

- The codespace stops after about 30 minutes without activity *from you* (not
  from your colleague), and the app with it. To keep it up longer, set
  **Settings › Codespaces › Default idle timeout** on GitHub (up to 4 hours),
  or keep the codespace open.
- To start it again: GitHub › **Code** › **Codespaces** › open it. The data is
  kept; the port goes back to private, so set it to **Public** again.
- Free accounts get about 30 hours a month on a 4-core machine. Stop the
  codespace when nobody needs it (**…** › **Stop codespace**).

## New versions

In the browser editor for your Codespace, open the Command Palette
(**Ctrl+Shift+P** on Windows or **Cmd+Shift+P** on Mac). Choose
**Tasks: Run Task**, then **Update HyPlug from main**. This pulls the
latest changes and rebuilds HyPlug in the same Codespace, keeping its
database and settings.

You can also run the updater from the Codespace terminal.

```bash
bash .devcontainer/update.sh
```

If the task is missing in a Codespace created before this update, run
`git pull --ff-only && bash .devcontainer/start.sh` once from the `main`
checkout. Reopening the Codespace starts its current checkout. Run the update
task when you want to pull a newer version.

## Safety

The codespace makes its own random secrets on the first start (kept outside
the repository, in `/workspaces/.hiringplug-codespace.env`), so the
development secrets published in the repository do not work there. Only
people who sign in can see anything; a public port only shows the sign-in
page. Delete the codespace when the trial is over.
