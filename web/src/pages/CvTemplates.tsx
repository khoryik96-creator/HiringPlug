import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { getToken } from "../auth";
import { ErrorMessage } from "../components/ErrorMessage";
import { cvTemplatesKey } from "../components/FormatCv";
import { SaveError } from "../components/SaveError";
import { formatWhen } from "../labels";
import { useRequiredFile } from "../requiredFields";

const DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";

/** The company's CV templates: start from the standard one, upload your own. */
export function CvTemplates({ canManage }: { canManage: boolean }) {
  const queryClient = useQueryClient();
  const list = useQuery({ queryKey: [...cvTemplatesKey, "full"], queryFn: async () => unwrap(await api.GET("/v1/cv-templates")) });
  const [name, setName] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const fileField = useRequiredFile(file);
  const refresh = async () => { await queryClient.invalidateQueries({ queryKey: cvTemplatesKey }); };
  const upload = useMutation({
    mutationFn: async () => {
      const body = new FormData();
      body.append("name", name);
      if (file) body.append("file", file);
      const res = await fetch("/v1/cv-templates", { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
      if (!res.ok) unwrap({ error: await res.json(), response: res });
    },
    onSuccess: async () => {
      setName("");
      setFile(null);
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: async (id: string) => { await api.DELETE("/v1/cv-templates/{cvTemplateId}", { params: { path: { cvTemplateId: id } } }); },
    onSettled: refresh,
  });
  const download = useMutation({
    mutationFn: async (t: { id: string; name: string }) => { await downloadFrom(`/v1/cv-templates/${t.id}/file`, `${t.name}.docx`, DOCX); },
  });
  return (
    <main>
      <h2>CV templates</h2>
      <ErrorMessage error={list.error ?? remove.error ?? download.error} />
      {list.data && (
        <section className="panel" aria-label="How templates work">
          <p>
            A template is a Word file with placeholders where the candidate&apos;s details go. Download the standard template, change its
            look in Word (logo, fonts, colours, layout), keep the placeholders you want, and upload it.
          </p>
          <p className="muted">
            Placeholders: {list.data.placeholders.map((p) => <code key={p}>{`{{${p}}}`} </code>)}
          </p>
          <p className="muted">
            Repeating blocks, each marker on its own line: {list.data.blocks.map((b) => <code key={b}>{b} </code>)}{" "}
            (between <code>{"{{#experience}}"}</code> and <code>{"{{/experience}}"}</code>). <code>{"{{#experience?}}"}</code> …{" "}
            <code>{"{{/experience?}}"}</code> keeps a heading only when there is something under it. A line whose placeholders are all
            empty is left out.
          </p>
        </section>
      )}
      <table>
        <thead><tr><th>Template</th><th>Uploaded</th><th /></tr></thead>
        <tbody>
          {list.data?.items.map((t) => (
            <tr key={t.id}>
              <td>{t.name}</td>
              <td className="muted">{t.builtIn ? "Built in" : <>{t.uploadedByName}{t.createdAt && <> · {formatWhen(t.createdAt)}</>}</>}</td>
              <td>
                <button type="button" className="link" onClick={() => { download.mutate(t); }}>Download</button>
                {canManage && !t.builtIn && (
                  <button type="button" className="link" onClick={() => { if (window.confirm(`Remove ${t.name}?`)) remove.mutate(t.id); }}>Remove</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {canManage && (
        <form className="panel form-grid" aria-label="Upload a template" onSubmit={(e) => { e.preventDefault(); upload.mutate(); }}>
          <label className="field">Name<input required value={name} onChange={(e) => { setName(e.target.value); }} /></label>
          <label className="field">Word file (.docx)<input ref={fileField} type="file" accept=".docx" onChange={(e) => { setFile(e.target.files?.[0] ?? null); }} /></label>
          <SaveError error={upload.error} shown={[]} onReload={() => undefined} />
          <div className="row full"><button type="submit" disabled={upload.isPending}>Upload</button></div>
        </form>
      )}
    </main>
  );
}
