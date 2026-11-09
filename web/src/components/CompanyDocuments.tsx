// Documents kept with a client's terms of business or benefits (the signed
// terms, a benefits brochure): drop or choose files, download them, and
// delete them for those given companies.documents:delete (Settings › Roles).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { getToken } from "../auth";
import { formatWhen } from "../labels";
import { CV_TYPES, DropZone } from "./DropZone";
import { ErrorMessage } from "./ErrorMessage";

type Doc = Schemas["CompanyDocument"];
type Section = Doc["section"];

export const companyDocumentsKey = (id: string) => ["company", id, "documents"] as const;

const KIND: Record<string, string> = {
  "application/pdf": "PDF", "application/msword": "DOC", "application/rtf": "RTF", "text/plain": "TXT",
  "application/vnd.oasis.opendocument.text": "ODT",
};
const kindOf = (d: Doc) => KIND[d.contentType] ?? (d.contentType.startsWith("image/") ? "IMG" : "DOCX");
const sizeOf = (bytes: number) => (bytes < 1024 * 1024 ? `${String(Math.max(1, Math.round(bytes / 1024)))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`);

export function CompanyDocuments({ companyId, section, permissions }: { companyId: string; section: Section; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const key = companyDocumentsKey(companyId);
  const docs = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/documents", { params: { path: { companyId } } })).items,
  });
  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      for (const file of files) {
        const body = new FormData();
        body.append("section", section);
        body.append("file", file);
        const res = await fetch(`/v1/companies/${companyId}/documents`, {
          method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` },
        });
        if (!res.ok) unwrap({ error: await res.json(), response: res });
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  });
  const remove = useMutation({
    mutationFn: async (d: Doc) => {
      unwrap(await api.DELETE("/v1/companies/{companyId}/documents/{documentId}", { params: { path: { companyId, documentId: d.id } } }));
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  });
  const download = useMutation({
    mutationFn: (d: Doc) => downloadFrom(`/v1/companies/${companyId}/documents/${d.id}/file`, d.fileName, d.contentType),
  });

  const mine = (docs.data ?? []).filter((d) => d.section === section);
  const canAdd = permissions.includes("companies:update");
  const canDelete = permissions.includes("companies.documents:delete");
  const label = section === "terms" ? "Terms documents" : "Benefits documents";

  return (
    <div className="company-docs" role="group" aria-label={label}>
      <h4>Documents</h4>
      <ErrorMessage error={docs.error ?? upload.error ?? remove.error ?? download.error} />
      {mine.length > 0 ? (
        <ul className="company-doc-list">
          {mine.map((d) => (
            <li key={d.id} className="doc-line">
              <span className={`doc-icon ${kindOf(d).toLowerCase()}`} aria-hidden="true">{kindOf(d)}</span>
              <div className="doc-main">
                <button type="button" className="doc-name" title="Download" onClick={() => { download.mutate(d); }}>{d.fileName}</button>
                <div className="doc-meta">{sizeOf(d.sizeBytes)} · added {formatWhen(d.createdAt)}{d.createdByName ? ` by ${d.createdByName}` : ""}</div>
              </div>
              <div className="doc-buttons">
                <button type="button" className="doc-btn" onClick={() => { download.mutate(d); }}>Download</button>
                {canDelete && (
                  <button type="button" className="doc-btn danger" aria-label={`Delete ${d.fileName}`} disabled={remove.isPending}
                    onClick={() => { if (window.confirm(`Delete ${d.fileName}? This cannot be undone.`)) remove.mutate(d); }}>
                    Delete
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      ) : (
        !canAdd && <p className="muted small">No documents.</p>
      )}
      {canAdd && (
        <DropZone accept={CV_TYPES} multiple disabled={upload.isPending}
          label={upload.isPending ? "Uploading…" : section === "terms" ? "Drop the signed terms here" : "Drop a benefits document here"}
          hint="PDF, Word, text or a picture" onFiles={(files) => { upload.mutate(files); }} />
      )}
    </div>
  );
}
