import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Document, unwrap } from "../api/client";
import { CvDifferences as Differences } from "../components/CvDifferences";
import { CvPane as Pane, useDocument } from "../components/CvPane";
import { ErrorMessage } from "../components/ErrorMessage";

/** Two pre-selected versions: the original against another source when there is one. */
function defaults(docs: Document[]): [string | undefined, string | undefined] {
  const original = docs.find((d) => d.source === "original") ?? docs[0];
  const other = docs.find((d) => d.id !== original?.id && d.source !== original?.source) ?? docs.find((d) => d.id !== original?.id);
  return [original?.id, other?.id];
}

/** A candidate's CV versions side by side, with what their details disagree on. */
export function CompareCvs({ candidateId }: { candidateId: string }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const [picked, setPicked] = useState<[string | undefined, string | undefined] | null>(null);
  const list = docs.data ?? [];
  const [leftId, rightId] = picked ?? defaults(list);
  const left = useDocument(leftId);
  const right = useDocument(rightId);

  if (docs.error) return <ErrorMessage error={docs.error} />;
  if (!docs.data) return <p className="muted">Loading…</p>;
  return (
    <main>
      <a href={`#/candidates/${candidateId}`}>← Candidate</a>
      <h2>Compare CVs</h2>
      {list.length < 2 && <p className="muted">Only one CV: upload another to compare.</p>}
      <Differences a={left.data} b={right.data} />
      <div className="compare">
        {[leftId, rightId].map((id, side) => (
          <Pane
            key={side}
            docs={list}
            doc={(side === 0 ? left : right).data}
            selected={id}
            onSelect={(v) => { setPicked(side === 0 ? [v, rightId] : [leftId, v]); }}
          />
        ))}
      </div>
    </main>
  );
}
