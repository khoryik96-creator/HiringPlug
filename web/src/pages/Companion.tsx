// Settings › LinkedIn companion: download the browser extension that reads
// LinkedIn in the recruiter's own browser, and how to install and connect it.
import { useState } from "react";

import { companionAtLeast, companionCall, useCompanion } from "../companion";
import { companionCan } from "../components/linkedinCompany";
import "./companionPage.css";

export const COMPANION_DOWNLOAD = "/linkedin-companion.zip";
/** The version built with this HyPlug (companionZip.ts). */
export const COMPANION_LATEST = __COMPANION_VERSION__;
/** From this version the companion updates itself (its Update page). */
const SELF_UPDATING = "1.4.0";

/** latest: the version to offer (the one built with this HyPlug; tests choose another). */
export function Companion({ latest = COMPANION_LATEST }: { latest?: string }) {
  const companion = useCompanion();
  const [opened, setOpened] = useState<string | null>(null);
  const v = companion.version ?? "?";
  const behind = companion.ready === true && !companionAtLeast(companion.version, latest);
  const selfUpdating = companionAtLeast(companion.version, SELF_UPDATING);
  const status = companion.ready === null ? { tone: "muted", text: "Checking this browser…" }
    : !companion.ready ? { tone: "warning", text: "Not connected in this browser yet." }
      : !behind ? { tone: "ok", text: `Connected in this browser (version ${v}).` }
        : selfUpdating ? { tone: "warning", text: `Connected (version ${v}). Version ${latest} is ready: update it here.` }
          : { tone: "warning", text: `Connected, but version ${v} is ${companionCan(companion.version) ? "out of date" : "old"}: download it again and reload it (step 6) once more; from ${SELF_UPDATING} it updates itself.` };
  const update = () => {
    setOpened(null);
    companionCall<boolean>("openUpdater", {}, 10_000).then(
      () => { setOpened("The companion's update page opened in a new tab: click Update there."); },
      (e: unknown) => { setOpened(e instanceof Error ? e.message : "The companion did not answer."); },
    );
  };
  return (
    <main className="companion-page">
      <h2>LinkedIn companion</h2>
      <p className="muted">
        A small add-on for Chrome or Edge. It reads LinkedIn in your own browser, with your own LinkedIn sign-in, to check
        whether contacts still work at a company and to fill a new company in from its LinkedIn page. HyPlug&apos;s server
        never reads LinkedIn, and the companion sends no messages.
      </p>
      <section className="panel companion-card" aria-label="Download">
        <p className={`companion-status ${status.tone}`} role="status">{status.text}</p>
        {behind && selfUpdating && <button type="button" className="pill-btn primary" onClick={update}>⟳ Update the companion</button>}
        {opened && <p className="muted small" role="status">{opened}</p>}
        <a className="pill-btn primary companion-download" href={COMPANION_DOWNLOAD} download="hiringplug-linkedin-companion.zip">
          ⬇ Download the companion
        </a>
        <span className="muted small">A small .zip file (about 35 KB). Each person installs it once in their own browser.</span>
      </section>
      <section className="panel" aria-labelledby="companion-steps">
        <h3 id="companion-steps">Install it (about 2 minutes)</h3>
        <ol className="companion-steps">
          <li><strong>Unzip</strong> the download (double-click it). Keep the <code>hiringplug-linkedin-companion</code> folder somewhere it can stay, e.g. Documents: the browser runs it from there.</li>
          <li>In Chrome open <code>chrome://extensions</code> (in Edge, <code>edge://extensions</code>) in the address bar.</li>
          <li>Turn on <strong>Developer mode</strong> (top right in Chrome, left side in Edge), then click <strong>Load unpacked</strong> and choose the unzipped folder.</li>
          <li>Click the puzzle-piece icon beside the address bar and <strong>pin</strong> HyPlug LinkedIn Companion, so its icon stays in view.</li>
          <li>Sign in to LinkedIn in this browser. Then come back to this HyPlug tab, click the companion&apos;s icon and <strong>Connect this HyPlug</strong>; allow it when the browser asks. The status above turns to “Connected”.</li>
          <li>To <strong>update</strong> it later: <strong>Update the companion</strong> here (it shows when a newer version is ready), or <em>Check for an update</em> under the companion&apos;s icon. The first time, choose the companion&apos;s folder and allow editing it; after that it is one click, and it restarts by itself. (A companion older than {SELF_UPDATING}: download it again, replace the folder&apos;s contents and click ↻ on its card in the extensions page, this last time.)</li>
        </ol>
        <p className="muted small">Once connected: on a company&apos;s contacts, <em>Check employment on LinkedIn</em>; adding a company with its LinkedIn page, <em>Fill in from LinkedIn</em>.</p>
      </section>
    </main>
  );
}
