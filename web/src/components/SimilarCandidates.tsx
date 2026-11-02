import { useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";

const REASON = { same_phone: "same phone", similar_name: "similar name" } as const;

/** A warning listing possible duplicates of a candidate; nothing if there are none. */
export function SimilarCandidates({ candidateId }: { candidateId: string }) {
  const similar = useQuery({
    queryKey: ["candidate", candidateId, "similar"],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/similar", { params: { path: { candidateId } } })).items,
  });
  if (!similar.data || similar.data.length === 0) return null;
  return (
    <section className="panel warning-panel" aria-label="Possible duplicates">
      <strong>Possible duplicate{similar.data.length === 1 ? "" : "s"}:</strong>
      <ul>
        {similar.data.map((s) => (
          <li key={s.candidate.id}>
            <a href={`#/candidates/${s.candidate.id}`}>{s.candidate.fullName}</a>{" "}
            <span className="muted">
              ({s.reasons.map((r) => REASON[r]).join(", ")}{s.candidate.currentTitle ? `; ${s.candidate.currentTitle}` : ""}{s.candidate.email ? `; ${s.candidate.email}` : ""})
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
