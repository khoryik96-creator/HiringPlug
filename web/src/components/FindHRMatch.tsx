// Find HR › In HyPlug: who a person found is (the same LinkedIn profile)
// or may be (the same or a similar name), anywhere in the agency, with that
// contact's company and email, so their work email is known before anything
// is looked up.
import type { MouseEvent } from "react";

import type { Schemas } from "../api/client";
import { CopyText } from "./CopyButton";

type Person = Schemas["HrScanPerson"];
type Match = Schemas["HrScanMatch"];

const KIND: Record<Match["kind"], { label: string; tip: string }> = {
  profile: { label: "In HyPlug", tip: "A contact with this LinkedIn profile" },
  same_name: { label: "Same name", tip: "A contact with the same name, in any order (titles and bin/binti aside): may be them" },
  similar_name: { label: "Similar name", tip: "A contact whose name is inside this one, or this one inside theirs: may be them" },
};

const CHECK: Partial<Record<NonNullable<Match["emailCheckStatus"]>, string>> = {
  invalid: "does not exist", risky: "may not arrive",
};

const stop = (e: MouseEvent) => { e.stopPropagation(); };

/** Their work email is in HyPlug already: this profile, at this company, a work (not free mail) address. */
export const workEmailKnown = (p: Person) =>
  !!p.match && p.match.kind === "profile" && p.match.sameCompany && !!p.match.email && !p.match.emailPersonal;

/** The "In HyPlug" cell: New, or the contact they are or may be, with their email. */
export function HRMatch({ person }: { person: Person }) {
  const m = person.match;
  if (!m) return <span className="hr-chip new">New</span>;
  const kind = KIND[m.kind];
  const where = [m.title, m.sameCompany ? null : `at ${m.companyName}`].filter(Boolean).join(" · ");
  return (
    <div className="hr-match">
      <a className={`hr-chip ${m.kind === "profile" ? "have" : "dup"}`} href={`#/contacts/${m.contactId}`} title={kind.tip} onClick={stop}>{kind.label}</a>
      {m.kind !== "profile" && <a className="hr-match-name" href={`#/contacts/${m.contactId}`} onClick={stop}>{m.name}</a>}
      {(where || m.left) && <div className="muted small">{where}{m.left && <>{where ? " · " : ""}<span className="hr-match-left">left</span></>}</div>}
      {m.email ? (
        <div className="hr-match-email small" onClick={stop}>
          <CopyText text={m.email} what="email" />
          {m.emailPersonal && <span className="muted"> personal</span>}
          {m.emailCheckStatus && CHECK[m.emailCheckStatus] && <span className="hr-match-warn"> {CHECK[m.emailCheckStatus]}</span>}
        </div>
      ) : <div className="muted small">No email saved</div>}
      {m.more > 0 && <div className="muted small">{m.more === 1 ? "1 more contact may be them" : `${String(m.more)} more contacts may be them`}</div>}
    </div>
  );
}

/** How many found are in HyPlug, may be, or are new. */
export function HRMatchSummary({ people }: { people: Person[] }) {
  const known = people.filter((p) => p.match?.kind === "profile").length;
  const maybe = people.filter((p) => p.match && p.match.kind !== "profile").length;
  const emails = people.filter((p) => p.match?.email).length;
  return (
    <span className="muted small hr-match-summary">
      {known} in HyPlug · {maybe} may be · {people.length - known - maybe} new{emails > 0 && <> · {emails} with an email</>}
    </span>
  );
}
