// A contact's employment check: drop their LinkedIn profile saved as PDF, and
// see whether they still work at the company they are recorded at.
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { type Schemas, unwrap } from "../api/client";
import { getToken } from "../auth";
import { contactKey } from "../api/keys";
import { formatWhen } from "../labels";
import { DropZone } from "./DropZone";
import { ErrorMessage } from "./ErrorMessage";

type Contact = Schemas["Contact"];
type Employment = Schemas["ContactEmployment"];
type Position = Schemas["ContactPosition"];

async function sendProfile(contactId: string, file: File): Promise<Employment> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(`/v1/contacts/${contactId}/linkedin-profile`, { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  const json: unknown = await res.json();
  if (!res.ok) unwrap({ error: json, response: res });
  return json as Employment;
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
/** "2026-04" → "Apr 2026"; "2026" stays. */
export function monthYear(d: string | null): string {
  if (!d) return "";
  const [y, m] = d.split("-");
  return m ? `${MONTHS[Number(m) - 1] ?? m} ${y ?? ""}` : (y ?? "");
}

const role = (p: Position) => [p.title, p.company].filter(Boolean).join(" at ");
const span = (p: Position) => `${monthYear(p.startDate)}${p.startDate ? " – " : ""}${p.current ? "now" : monthYear(p.endDate)}`;

export function EmploymentCheck({ contact, canCheck }: { contact: Contact; canCheck: boolean }) {
  const queryClient = useQueryClient();
  const upload = useMutation({
    mutationFn: (file: File) => sendProfile(contact.id, file),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: contactKey(contact.id) }); },
  });
  const e = contact.employment;
  if (!e && !canCheck) return null;
  return (
    <section className="panel employment" aria-label="Employment check">
      <div className="row spread">
        <h3>Employment check</h3>
        {e && <span className="muted small">LinkedIn, {formatWhen(e.checkedAt)}</span>}
      </div>
      {e && <Result e={e} contact={contact} />}
      {canCheck && (
        <DropZone accept=".pdf" disabled={upload.isPending || e?.status === "reading"}
          label={e ? "Drop a newer LinkedIn PDF" : "Drop their LinkedIn profile PDF"}
          hint="On their LinkedIn profile: More › Save to PDF"
          onFiles={([f]) => { if (f) upload.mutate(f); }} />
      )}
      <ErrorMessage error={upload.error} />
    </section>
  );
}

function Result({ e, contact }: { e: Employment; contact: Contact }) {
  if (e.status === "reading") return <p className="muted" role="status">Reading {e.fileName}…</p>;
  if (e.status === "failed") return <p className="warning">{e.error ?? "It could not be read."}</p>;
  const cur = e.current;
  return (
    <>
      {e.verdict === "same_company" && cur && (
        <p className="employment-verdict ok"><strong>Still at {contact.companyName}</strong> · {role(cur)}{cur.startDate && `, since ${monthYear(cur.startDate)}`}</p>
      )}
      {e.verdict === "moved" && cur && (
        <p className="employment-verdict moved"><strong>No longer at {contact.companyName}</strong> · now {role(cur)}{cur.startDate && `, since ${monthYear(cur.startDate)}`}</p>
      )}
      {e.verdict === "no_current_job" && (
        <p className="employment-verdict moved"><strong>No current job on their profile</strong> · they may have left {contact.companyName}</p>
      )}
      {e.verdict === "unknown" && <p className="muted">Their current job does not name a company.</p>}
      {(e.nameMatches === false || e.urlMatches === false) && (
        <p className="warning small">
          This may be someone else&apos;s profile: {e.nameMatches === false ? `the name on it is ${e.profileName ?? "different"}` : "its LinkedIn address differs from the contact's"}.
        </p>
      )}
      {e.positions.length > 0 && (
        <ul className="employment-history">
          {e.positions.map((p, i) => (
            <li key={i} className={p.current ? "current" : undefined}><span>{role(p)}</span><span className="muted">{span(p)}</span></li>
          ))}
        </ul>
      )}
    </>
  );
}
