// The candidate's CV beside their details (the "CV beside" switch on the
// candidate's page): the newest CV they sent, with a version picker. It stays
// in view while the details scroll.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { CvPane, latestCv, useDocument } from "./CvPane";
import { ErrorMessage } from "./ErrorMessage";

export function CvBeside({ candidateId }: { candidateId: string }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const [picked, setPicked] = useState<string | undefined>();
  const list = docs.data ?? [];
  const selected = picked ?? latestCv(list)?.id;
  const doc = useDocument(selected);
  return (
    <div className="cv-beside" aria-label="CV beside the details" role="region">
      {docs.error ? <ErrorMessage error={docs.error} />
        : !docs.data ? <p className="panel muted">Loading…</p>
        : list.length === 0 ? <p className="panel muted">No CV yet. Add one in CV &amp; files.</p>
        : <CvPane docs={list} doc={doc.data} selected={selected} onSelect={setPicked} />}
    </div>
  );
}

const KEY = "hp.cvBeside";
/** Whether the CV is shown beside the details, remembered in this browser. */
export function useCvBeside(): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(() => { try { return localStorage.getItem(KEY) === "on"; } catch { return false; } });
  return [on, (next) => {
    setOn(next);
    try { localStorage.setItem(KEY, next ? "on" : "off"); } catch { /* private window: not remembered */ }
  }];
}
