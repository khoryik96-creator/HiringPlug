// Candidates' details in an Outlook message, as HiringPlug's client email
// writes them: a block each (or one table) with the details chosen, salary
// included for roles that may see salaries, put where the cursor is.
import type { Permission } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { ShortlistPanel, useShortlist } from "../clientEmail/Shortlist";
import type { useCandidatePackages } from "../clientEmail/useCandidatePackages";

export function OutlookCandidateDetails({ packages, permissions }: {
  packages: ReturnType<typeof useCandidatePackages>; permissions: Permission[];
}) {
  const list = useShortlist(packages.people, false);
  return (
    <details className="outlook-candidate-details" open={packages.people.length > 0 || undefined}>
      <summary>Candidate details{packages.people.length ? ` (${String(packages.people.length)})` : ""}: salary, notice period…</summary>
      <ShortlistPanel people={packages.people} onPeople={packages.change} list={list} canSeeCvs={false} attachProfiles={false} allowEmpty
        canSeeSalary={permissions.includes("candidates.compensation:read")} onRefresh={packages.clear} onInsert={packages.insert} />
      {packages.notice && <p className="muted small" role="status">{packages.notice}</p>}
      <ErrorMessage error={list.error} />
    </details>
  );
}
