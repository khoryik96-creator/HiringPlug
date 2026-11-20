// Candidate CV choices share the Word and PDF variants used by client email.
import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "../../api/client";
import { fetchDocumentFile, fetchDocumentPdf } from "../../api/files";
import { choiceKey, profilesOf } from "../clientEmail/shortlistHtml";
import { ErrorMessage } from "../ErrorMessage";

export type AttachCandidateCv = (loadFile: () => Promise<File>) => void;

export function AttachCv({ candidateId, name, onAttach }: { candidateId: string; name: string; onAttach: AttachCandidateCv }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  if (docs.isPending) return <span className="muted small">…</span>;
  if (docs.error) return <ErrorMessage error={docs.error} />;
  if (!docs.data.length) return <span className="muted small">No CV on record</span>;
  const profiles = profilesOf(docs.data);
  return <select aria-label={`Attach a CV of ${name}`} value="" onChange={(event) => {
    const choice = profiles.find((p) => choiceKey(p.choice) === event.target.value)?.choice;
    const doc = docs.data.find((d) => d.id === choice?.documentId);
    if (!doc || !choice) return;
    onAttach(async () => {
      const blob = choice.asPdf ? await fetchDocumentPdf(doc.id) : await fetchDocumentFile(doc);
      const filename = choice.asPdf ? doc.fileName.replace(/\.docx?$/i, "") + ".pdf" : doc.fileName;
      return new File([blob], filename, { type: blob.type });
    });
  }}>
    <option value="">📎 Attach CV…</option>
    {profiles.map(({ choice, label }) => <option key={choiceKey(choice)} value={choiceKey(choice)}>{label}</option>)}
  </select>;
}
