// Settings > System: the server's own checks, for the agency that runs the
// server. Whether the background worker and the AI worker run, and on this
// version (one left on the old version after an update shows here); whether
// files can be kept and opened; the disk; and what the alert emails report.
import type { Schemas } from "../api/client";
import { formatWhen } from "../labels";

type Checks = Schemas["ServerChecks"];
type Process = Schemas["ServerProcess"];

const NAMES: Record<Process["name"], string> = { worker: "Background worker", "ai-worker": "AI worker" };
const WORK: Record<Process["name"], string> = {
  worker: "emails, calendar updates, reminders and automations wait",
  "ai-worker": "CVs and job descriptions are not read, and AI work waits",
};

type Look = "ok" | "warn" | "bad";

function processRow(p: Process, version: string): { look: Look; state: string; detail: string } {
  switch (p.state) {
    case "running":
      return { look: "ok", state: "Running", detail: `Version ${p.version ?? "?"}, started ${p.startedAt ? formatWhen(p.startedAt) : "?"}.` };
    case "old_version":
      return { look: "bad", state: "Old version", detail: `Runs ${p.version ?? "?"}, the rest of HyPlug ${version}: restart it after the update.` };
    case "stopped":
      return { look: "bad", state: "Stopped", detail: `Last checked in ${p.seenAt ? formatWhen(p.seenAt) : "?"}: ${WORK[p.name]}.` };
    case "never_seen":
      return { look: "warn", state: "Not seen", detail: "It has never checked in: it is not running here, or runs a version before 1.52.0." };
  }
}

const gb = (n: number) => `${(n / 1024 ** 3).toFixed(1)} GB`;

function diskRow(free: number | null, total: number | null): { look: Look; state: string; detail: string } {
  if (free === null || total === null) return { look: "warn", state: "Unknown", detail: "The server could not tell its free space." };
  const low = free < total / 10 || free < 2 * 1024 ** 3;
  return { look: low ? "bad" : "ok", state: low ? "Nearly full" : "OK", detail: `${gb(free)} free of ${gb(total)}.` };
}

const BADGE: Record<Look, string> = { ok: "badge check-ok", warn: "badge check-warn", bad: "badge check-bad" };

export function ServerChecks({ checks }: { checks: Checks }) {
  const rows = [
    ...checks.processes.map((p) => ({ name: NAMES[p.name], ...processRow(p, checks.version) })),
    checks.files === null
      ? { name: "Files folder", look: "ok" as Look, state: "OK", detail: "Files can be saved and opened." }
      : { name: "Files folder", look: "bad" as Look, state: "Problem", detail: `Files cannot be saved or opened: ${checks.files}` },
    { name: "Disk", ...diskRow(checks.freeBytes, checks.totalBytes) },
  ];
  const allWell = checks.problems.length === 0 && rows.every((r) => r.look === "ok");
  return (
    <section className="panel server-checks" aria-label="Server checks">
      <h3>Server checks</h3>
      <p className={allWell ? "muted" : "warning"} role="status">
        {allWell ? `All is well. HyPlug ${checks.version}.` : `Something needs looking at. HyPlug ${checks.version}.`}
      </p>
      <div className="table-scroll">
        <table>
          <thead><tr><th>Check</th><th>State</th><th>Details</th></tr></thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.name}>
                <td>{r.name}</td>
                <td><span className={BADGE[r.look]}>{r.state}</span></td>
                <td className="muted">{r.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {checks.problems.length > 0 && (
        <>
          <h4>What the alert emails report now</h4>
          <ul className="server-problems">
            {checks.problems.map((p) => <li key={p}>{p}</li>)}
          </ul>
        </>
      )}
    </section>
  );
}
