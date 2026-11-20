// Candidates' details in an Outlook message, as HiringPlug's client email
// writes them: a block each (or one table) with the details chosen, salary
// included for roles that may see salaries, put where the cursor is; and
// each candidate's CV, attached to the message from their row.
import type { Permission } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { AttachCv, type AttachCandidateCv } from "./AttachCv";
import { ShortlistPanel, useShortlist } from "../clientEmail/Shortlist";
import type { useCandidatePackages } from "../clientEmail/useCandidatePackages";

export function OutlookCandidateDetails({ packages, permissions, onAttach }: {
  packages: ReturnType<typeof useCandidatePackages>; permissions: Permission[]; onAttach: AttachCandidateCv;
}) {
  const canAttach = permissions.includes("documents:read");
  const list = useShortlist(packages.people, false);
  return (
    <details className="outlook-candidate-details" open={packages.people.length > 0 || undefined}>
      <summary>Candidate details{canAttach ? " & CVs" : ""}{packages.people.length ? ` (${String(packages.people.length)})` : ""}: salary, notice period{canAttach ? ", CV to attach" : ""}…</summary>
      <ShortlistPanel people={packages.people} onPeople={packages.change} list={list} canSeeCvs={false} attachProfiles={false} allowEmpty
        rowTool={canAttach ? (p, name) => <AttachCv candidateId={p.candidateId} name={name} onAttach={onAttach} /> : undefined}
        canSeeSalary={permissions.includes("candidates.compensation:read")} onRefresh={packages.clear} onInsert={packages.insert} />
      {packages.notice && <p className="muted small" role="status">{packages.notice}</p>}
      <ErrorMessage error={list.error} />
    </details>
  );
}
