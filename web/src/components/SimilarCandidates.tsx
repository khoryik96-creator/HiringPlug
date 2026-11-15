import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

const REASON = { same_phone: "same phone", similar_name: "similar name" } as const;

/**
 * A warning listing possible duplicates of a candidate (nothing if none).
 * With candidates:merge, a duplicate can be merged into this candidate.
 */
export function SimilarCandidates({ candidate, canMerge }: { candidate: Candidate; canMerge: boolean }) {
  const queryClient = useQueryClient();
  const similarKey = ["candidate", candidate.id, "similar"] as const;
  const similar = useQuery({
    queryKey: similarKey,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/similar", { params: { path: { candidateId: candidate.id } } })).items,
  });
  const merge = useMutation({
    mutationFn: async (duplicateId: string) => unwrap(await api.POST("/v1/candidates/{candidateId}/merge", {
      params: { path: { candidateId: candidate.id } }, body: { duplicateId, version: candidate.version },
    })),
    onSuccess: (merged) => {
      queryClient.setQueryData(candidateKey(candidate.id), merged);
      void queryClient.invalidateQueries(); // CVs, jobs, notes and lists all changed
    },
  });
  const [open, setOpen] = useState(false);
  if (!similar.data || similar.data.length === 0) return null;
  const n = similar.data.length;
  const why = [...new Set(similar.data.flatMap((s) => s.reasons))].map((r) => REASON[r]).join(" or ");
  return (
    <section className="dupe-line" aria-label="Possible duplicates">
      <div className="dupe-head">
        <span className="dupe-icon" aria-hidden="true">!</span>
        <span className="dupe-text"><strong>{n === 1 ? "1 possible duplicate" : `${String(n)} possible duplicates`}</strong> <span className="muted">· {why}</span></span>
        <button type="button" className="small-button" aria-expanded={open} onClick={() => { setOpen(!open); }}>
          {open ? "Hide" : canMerge ? "Review and merge" : "Review"}
        </button>
      </div>
      {open && <ul>
        {similar.data.map((s) => (
          <li key={s.candidate.id}>
            <a href={`#/candidates/${s.candidate.id}`}>{s.candidate.fullName}</a>{" "}
            <span className="muted">
              ({s.reasons.map((r) => REASON[r]).join(", ")}{s.candidate.currentTitle ? `; ${s.candidate.currentTitle}` : ""}{s.candidate.email ? `; ${s.candidate.email}` : ""})
            </span>{" "}
            {canMerge && (
              <button
                type="button"
                className="link"
                disabled={merge.isPending}
                onClick={() => {
                  if (window.confirm(`Merge ${s.candidate.fullName} into ${candidate.fullName}? Their CVs, jobs and notes move here, empty fields here are filled from theirs, and ${s.candidate.fullName} is deleted.`)) {
                    merge.mutate(s.candidate.id);
                  }
                }}
              >
                Merge into this one
              </button>
            )}
          </li>
        ))}
      </ul>}
      <ErrorMessage error={merge.error} />
    </section>
  );
}
