import { ApiError } from "../api/client";

/** Who already has an email (one candidate per email), with a link to them, inside an error. */
export function DuplicateEmail({ error }: { error: unknown }) {
  if (!(error instanceof ApiError) || error.code !== "CANDIDATE_DUPLICATE_EMAIL") return null;
  const id = typeof error.details.existingCandidateId === "string" ? error.details.existingCandidateId : null;
  const name = typeof error.details.existingCandidateName === "string" ? error.details.existingCandidateName : null;
  const owner = typeof error.details.existingOwnerName === "string" ? error.details.existingOwnerName : null;
  if (!id) return null; // someone you may not see
  return (
    <p>
      <a href={`#/candidates/${id}`}>{name ?? "Open that candidate"}</a>
      {owner ? ` (owned by ${owner})` : ""} already has it. Add the CV to them instead.
    </p>
  );
}
