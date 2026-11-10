import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { Assistant } from "../components/Assistant";
import { CvPane, latestCv, useDocument } from "../components/CvPane";
import { ErrorMessage } from "../components/ErrorMessage";
import { ScreeningCalls } from "../components/ScreeningCalls";
import { ScreeningForm } from "../components/ScreeningForm";

/** A screening call: the CV on one side, the call's notes and questions on the other, with the AI assistant. */
export function ScreeningCall({ candidateId, permissions }: { candidateId: string; permissions: Permission[] }) {
  const candidate = useQuery({
    queryKey: candidateKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId } } })),
  });
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    enabled: permissions.includes("documents:read"),
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const [picked, setPicked] = useState<string | undefined>();
  const list = docs.data ?? [];
  const selected = picked ?? latestCv(list)?.id;
  const doc = useDocument(selected);

  const c = candidate.data;
  if (!c) return candidate.error ? <ErrorMessage error={candidate.error} /> : <p className="muted">Loading…</p>;
  return (
    <main className="wide">
      <a href={`#/candidates/${candidateId}`}>← {c.fullName}</a>
      <h2>Screening call: {c.fullName}</h2>
      <div className="screening">
        <div className="screening-cv">
          {!permissions.includes("documents:read") ? <p className="muted">Your role may not see CVs.</p>
            : docs.error ? <ErrorMessage error={docs.error} />
            : !docs.data ? <p className="muted">Loading…</p>
            : list.length === 0 ? <p className="panel muted">No CV yet.</p>
            : <CvPane docs={list} doc={doc.data} selected={selected} onSelect={setPicked} />}
        </div>
        <div className="screening-side">
          {permissions.includes("candidates:update")
            ? <ScreeningForm key={c.id} candidate={c} permissions={permissions} />
            : <p className="muted">Your role may not update candidates.</p>}
          <ScreeningCalls candidateId={c.id} permissions={permissions} />
          <Assistant subjectType="candidate" subjectId={c.id} permissions={permissions} />
        </div>
      </div>
    </main>
  );
}
