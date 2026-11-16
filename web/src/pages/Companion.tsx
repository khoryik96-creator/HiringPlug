// Settings › LinkedIn companion: download the browser extension that reads
// LinkedIn in the recruiter's own browser, and how to install and connect it.
import { useCompanion } from "../companion";
import { companionCan } from "../components/linkedinCompany";
import "./companion.css";

export const COMPANION_DOWNLOAD = "/linkedin-companion.zip";

export function Companion() {
  const companion = useCompanion();
  const status = companion.ready === null ? { tone: "muted", text: "Checking this browser…" }
    : !companion.ready ? { tone: "warning", text: "Not connected in this browser yet." }
      : companionCan(companion.version) ? { tone: "ok", text: `Connected in this browser (version ${companion.version ?? "?"}).` }
        : { tone: "warning", text: `Connected, but version ${companion.version ?? "?"} is old: download it again and reload it (step 6).` };
  return (
    <main className="companion-page">
      <h2>LinkedIn companion</h2>
      <p className="muted">
        A small add-on for Chrome or Edge. It reads LinkedIn in your own browser, with your own LinkedIn sign-in, to check
        whether contacts still work at a company and to fill a new company in from its LinkedIn page. HiringPlug&apos;s server
        never reads LinkedIn, and the companion sends no messages.
      </p>
      <section className="panel companion-card" aria-label="Download">
        <p className={`companion-status ${status.tone}`} role="status">{status.text}</p>
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
          <li>Click the puzzle-piece icon beside the address bar and <strong>pin</strong> HiringPlug LinkedIn Companion, so its icon stays in view.</li>
          <li>Sign in to LinkedIn in this browser. Then come back to this HiringPlug tab, click the companion&apos;s icon and <strong>Connect this HiringPlug</strong>; allow it when the browser asks. The status above turns to “Connected”.</li>
          <li>To <strong>update</strong> it later: download it again, replace the folder&apos;s contents, and click the ↻ reload button on its card in the extensions page.</li>
        </ol>
        <p className="muted small">Once connected: on a company&apos;s contacts, <em>Check employment on LinkedIn</em>; adding a company with its LinkedIn page, <em>Fill in from LinkedIn</em>.</p>
      </section>
    </main>
  );
}
