// What goes with the email: the candidate's CVs from the ATS (a formatted CV
// as PDF too, once made) and files from the computer.
import { useMutation, useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { getToken } from "../../auth";
import { ErrorMessage } from "../ErrorMessage";

export type CvChoice = Schemas["EmailCvAttachment"];
type Upload = Schemas["EmailAttachment"];

const SOURCE: Record<string, string> = { original: "Original", jobstreet: "JobStreet", linkedin: "LinkedIn", formatted: "Formatted", other: "Other" };
const mb = (bytes: number) => `${(bytes / 1048576).toFixed(bytes < 104858 ? 2 : 1)} MB`;

export function Attachments({ candidateId, canSeeCvs, cvs, onCvs, uploads, onUploads, others = 0 }: {
  candidateId: string;
  /** Files chosen elsewhere (each candidate's profile, on an email about several). */
  others?: number;
  canSeeCvs: boolean;
  cvs: CvChoice[];
  onCvs: (v: CvChoice[]) => void;
  uploads: Upload[];
  onUploads: (v: Upload[]) => void;
}) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    enabled: canSeeCvs,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      const out: Upload[] = [];
      for (const f of files) {
        const body = new FormData();
        body.append("file", f);
        const res = await fetch("/v1/email-attachments", { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
        const data: unknown = await res.json();
        if (!res.ok) unwrap({ error: data, response: res });
        out.push(data as Upload);
      }
      return out;
    },
    onSuccess: (added) => { onUploads([...uploads, ...added]); },
  });
  const has = (id: string, asPdf: boolean) => cvs.some((c) => c.documentId === id && Boolean(c.asPdf) === asPdf);
  const toggle = (id: string, asPdf: boolean) => {
    onCvs(has(id, asPdf) ? cvs.filter((c) => !(c.documentId === id && Boolean(c.asPdf) === asPdf)) : [...cvs, { documentId: id, asPdf }]);
  };
  const count = cvs.length + uploads.length;

  return (
    <fieldset className="attachments">
      <legend>Attachments · up to 10 files, 20 MB in all ({count + others} chosen)</legend>
      {canSeeCvs && (
        <div>
          <span className="muted small">From the ATS</span>
          {docs.data?.length === 0 && <p className="muted small">This candidate has no CV yet.</p>}
          <ul className="plain">
            {docs.data?.map((d) => (
              <li key={d.id}>
                <label><input type="checkbox" checked={has(d.id, false)} onChange={() => { toggle(d.id, false); }} /> {SOURCE[d.source] ?? d.source}: {d.fileName} <span className="muted">({mb(d.sizeBytes)})</span></label>
                {d.source === "formatted" && d.pdfStatus === "ready" && (
                  <label> <input type="checkbox" checked={has(d.id, true)} onChange={() => { toggle(d.id, true); }} /> as PDF</label>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
      <div>
        <label className="row">
          <span className="muted small">From your computer</span>
          <input type="file" multiple aria-label="Attach files" onChange={(e) => {
            const files = [...(e.target.files ?? [])];
            if (files.length > 0) upload.mutate(files);
            e.target.value = "";
          }} />
        </label>
        {upload.isPending && <p className="muted small">Uploading…</p>}
        <ul className="plain">
          {uploads.map((u) => (
            <li key={u.id}>
              {u.fileName} <span className="muted">({mb(u.sizeBytes)})</span>{" "}
              <button type="button" className="link" onClick={() => { onUploads(uploads.filter((x) => x.id !== u.id)); }}>Remove</button>
            </li>
          ))}
        </ul>
        <ErrorMessage error={upload.error} />
      </div>
    </fieldset>
  );
}
