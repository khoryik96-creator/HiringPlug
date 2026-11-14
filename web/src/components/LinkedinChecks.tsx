import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { contactKey, contactsKey } from "../api/keys";
import { companionCall, operationId, useCompanion } from "../companion";
import { formatWhen, linkedinDates } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Row = { name: string; contactId: string; text: string; tone: "left" | "there" | "unclear" | "failed" | "waiting" };

interface Read { url: string; company_url: string; company_id: string; joined: string; left: string; current: boolean | null }

function rowFor(name: string, contactId: string, check: Schemas["ContactLinkedinCheck"]): Row {
  if (check.status !== "observed") return { name, contactId, tone: "failed", text: check.error ?? "Could not be read." };
  if (check.current === false) {
    return { name, contactId, tone: "left", text: `Left ${linkedinDates(check.left)}${check.markedLeft ? " · marked as Has left" : ""}` };
  }
  if (check.current) return { name, contactId, tone: "there", text: `Still there, since ${linkedinDates(check.joined)}` };
  return { name, contactId, tone: "unclear", text: "LinkedIn does not show clearly: nothing changed" };
}

/**
 * "Check employment on LinkedIn": for a company's contacts (or one contact),
 * the LinkedIn companion reads, in this browser, each one's time at the
 * company; those whose time there ended are marked as having left.
 */
export function LinkedinChecks(props: { companyId?: string; contactId?: string; label?: string }) {
  const { button, panel } = useLinkedinChecks(props);
  return <div className="linkedin-checks">{button}{panel}</div>;
}

/** The same, as its button and its results, to place apart (the button in a header). */
export function useLinkedinChecks({ companyId, contactId, label = "Check employment on LinkedIn" }: {
  companyId?: string; contactId?: string; label?: string;
}) {
  const queryClient = useQueryClient();
  const companion = useCompanion();
  const [rows, setRows] = useState<Row[]>([]);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [help, setHelp] = useState(false);
  const stop = useRef<{ stopped: boolean; operation: string } | null>(null);

  const run = async () => {
    setError(null);
    if (!companion.ready) { setHelp(true); return; }
    const token = { stopped: false, operation: "" };
    const stopped = () => token.stopped; // set by Stop while this runs
    stop.current = token;
    setRunning(true);
    try {
      const started = unwrap(await api.POST("/v1/contacts/linkedin-checks", {
        body: contactId ? { contactId } : { companyId: companyId ?? "" },
      }));
      setRows([
        ...started.checks.map((c): Row => ({ name: c.contactName, contactId: c.contactId, tone: "waiting", text: "Waiting…" })),
        ...started.skipped.map((s): Row => ({ name: s.contactName, contactId: s.contactId, tone: "unclear",
          text: s.reason === "too_many" ? "Not checked this time (over 200 at once): check again for the rest" : "No LinkedIn profile address" })),
      ]);
      if (started.checks.length === 0) return;
      token.operation = operationId();
      const company = await companionCall<{ company_id: string }>("companyId", { operation_id: token.operation, company_url: started.companyUrl });
      for (const check of started.checks) {
        if (stopped()) break;
        token.operation = check.id;
        setRows((rs) => rs.map((r) => (r.contactId === check.contactId ? { ...r, text: "Reading their LinkedIn profile…" } : r)));
        let body: Schemas["ContactLinkedinCheckAnswer"];
        try {
          const read = await companionCall<Read>("employment", {
            operation_id: check.id, url: check.profileUrl, company_url: started.companyUrl, company_id: company.company_id,
          });
          body = { profileUrl: read.url, companyUrl: read.company_url, linkedinCompanyId: read.company_id, joined: read.joined, left: read.left, current: read.current };
        } catch (e) {
          if (stopped()) break;
          body = { error: e instanceof Error ? e.message.slice(0, 300) : "Could not be read." };
        }
        const saved = unwrap(await api.POST("/v1/contacts/linkedin-checks/{checkId}/answer", { params: { path: { checkId: check.id } }, body }));
        setRows((rs) => rs.map((r) => (r.contactId === check.contactId ? rowFor(r.name, r.contactId, saved) : r)));
      }
    } catch (e) {
      setError(e);
    } finally {
      setRunning(false);
      stop.current = null;
      const why = token.stopped ? "Not checked (stopped)" : "Not checked (see the message above)";
      setRows((rs) => rs.map((r) => (r.tone === "waiting" ? { ...r, text: why } : r)));
      await queryClient.invalidateQueries({ queryKey: contactsKey });
      if (contactId) await queryClient.invalidateQueries({ queryKey: contactKey(contactId) });
    }
  };

  const halt = () => {
    const token = stop.current;
    if (!token) return;
    token.stopped = true;
    void companionCall("cancel", { operation_id: token.operation }, 5000).catch(() => undefined);
  };

  const button = running
    ? <button type="button" onClick={halt}>Stop checking LinkedIn</button>
    : <button type="button" onClick={() => { void run(); }}
        title="Your browser reads each contact's LinkedIn profile (you must be signed in to LinkedIn)">{label}</button>;
  const panel = (
    <>
      {help && !companion.ready && (
        <div className="info companion-help" role="status">
          <p><strong>Install the HiringPlug LinkedIn Companion first.</strong> It reads LinkedIn in this browser, with your LinkedIn sign-in; HiringPlug&apos;s server never does.</p>
          <ol>
            <li>In Chrome or Edge, open Extensions, turn on Developer mode and choose <em>Load unpacked</em>: the <code>extension/linkedin-companion</code> folder of HiringPlug.</li>
            <li>Sign in to LinkedIn in this browser.</li>
            <li>On this tab, click the companion&apos;s icon and <em>Connect this HiringPlug</em>, then try again.</li>
          </ol>
        </div>
      )}
      <ErrorMessage error={error} />
      {rows.length > 0 && (
        <ul className="linkedin-results" aria-label="LinkedIn employment">
          {rows.map((r) => (
            <li key={r.contactId} className={`tone-${r.tone}`}><strong>{r.name}</strong> — {r.text}</li>
          ))}
        </ul>
      )}
    </>
  );
  return { button, panel };
}

/** A contact's latest LinkedIn check, in words. */
export function LinkedinCheckResult({ check }: { check: Schemas["ContactLinkedinCheck"] }) {
  const row = rowFor("", check.contactId, check);
  return (
    <p className={`linkedin-result tone-${row.tone}`}>
      {row.text}
      {check.checkedAt && <span className="muted"> · checked {formatWhen(check.checkedAt)}</span>}
    </p>
  );
}
