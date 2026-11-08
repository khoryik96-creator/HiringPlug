// The candidate's CV, shown on their page: the newest one they sent, as it
// looks (a PDF, or a Word file's PDF copy), with their other CVs to pick from.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, unwrap } from "../api/client";
import { CvPane, latestCv, useDocument } from "./CvPane";
import { ErrorMessage } from "./ErrorMessage";

const WORD = new Set(["application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/msword"]);

/** highlight: a boolean search whose words to mark (opened from the search's results). */
export function CandidateCv({ candidateId, highlight = "" }: { candidateId: string; highlight?: string }) {
  const queryClient = useQueryClient();
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
    // Follow a Word file's preview still being made.
    refetchInterval: (q) => (q.state.data?.some((d) => d.pdfStatus === "queued") ? 3000 : false),
  });
  const [picked, setPicked] = useState<string | undefined>();
  const list = docs.data ?? [];
  const selected = picked ?? latestCv(list)?.id;
  const fromList = list.find((d) => d.id === selected);
  const doc = useDocument(selected);
  // A Word CV from before previews began gets its PDF copy made, once.
  const asked = useRef(new Set<string>());
  useEffect(() => {
    if (!fromList || !WORD.has(fromList.contentType) || fromList.pdfStatus !== null || asked.current.has(fromList.id)) return;
    asked.current.add(fromList.id);
    void api.POST("/v1/documents/{documentId}/pdf", { params: { path: { documentId: fromList.id } } })
      .then(() => queryClient.invalidateQueries({ queryKey: ["documents", candidateId] }));
  }, [fromList, candidateId, queryClient]);
  if (docs.error) return <ErrorMessage error={docs.error} />;
  if (list.length === 0) return null; // no CV yet: the documents panel below takes one
  // The list knows when a preview is ready; the document itself has the text.
  const shown = doc.data && fromList ? { ...doc.data, pdfStatus: fromList.pdfStatus } : doc.data;
  return (
    <div className="candidate-cv">
      <CvPane title="CV" docs={list} doc={shown} selected={selected} onSelect={setPicked} highlight={highlight} />
    </div>
  );
}
