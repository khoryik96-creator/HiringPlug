import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, unwrap } from "../../api/client";
import { fetchFile, uploadCandidateDocument } from "../../api/files";
import { candidateKey } from "../../api/keys";
import { getToken } from "../../auth";
import { ErrorMessage } from "../ErrorMessage";
import { Field } from "../Field";
import { type Picked, SearchPicker } from "../SearchPicker";

async function searchCandidates(text: string) {
  const page = unwrap(await api.GET("/v1/candidates", { params: { query: { q: text, limit: 8 } } }));
  return page.items.map((candidate) => ({
    id: candidate.id,
    name: candidate.fullName,
    detail: [candidate.email, candidate.currentTitle, candidate.currentCompany].filter(Boolean).join(" · "),
  }));
}

export function OutlookSaveCv({ fileName, filePath, candidateId, onClose }: {
  fileName: string;
  filePath: string;
  candidateId?: string;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [choice, setChoice] = useState<Picked | null>();
  const contextId = candidateId ?? "";
  const context = useQuery({
    queryKey: candidateKey(contextId),
    enabled: contextId !== "" && choice === undefined,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}", { params: { path: { candidateId: contextId } } })),
  });
  const profile = context.data;
  const selected = choice === undefined ? (profile ? { id: profile.id, name: profile.fullName } : null) : choice;
  const save = useMutation({
    mutationFn: async (candidate: Picked) => {
      const token = getToken();
      const file = await fetchFile(filePath, "application/octet-stream");
      if (getToken() !== token) throw new Error("Your sign-in changed. Open the message again to save this CV.");
      try {
        await uploadCandidateDocument(candidate.id, "original", file, fileName);
        return { candidate, duplicate: false };
      } catch (error) {
        if (error instanceof ApiError && error.code === "DOCUMENT_DUPLICATE") return { candidate, duplicate: true };
        throw error;
      }
    },
    onSuccess: ({ candidate }) => client.invalidateQueries({ queryKey: ["documents", candidate.id] }),
  });

  if (save.data) {
    const { candidate, duplicate } = save.data;
    return <div className="outlook-save-cv">
      <p role="status">{duplicate ? "This CV is already in" : "CV saved to"} {candidate.name}’s profile.</p>
      <div className="row">
        <a href={`#/candidates/${encodeURIComponent(candidate.id)}?tab=cv`}>Open candidate</a>
        <button type="button" onClick={onClose}>Done</button>
      </div>
    </div>;
  }

  return <form className="outlook-save-cv" aria-label={`Save ${fileName} to a candidate`} onSubmit={(event) => {
    event.preventDefault();
    if (selected && !save.isPending) save.mutate(selected);
  }}>
    <fieldset disabled={save.isPending}>
      <Field label="Candidate">
        <SearchPicker value={selected} onChange={(next) => { save.reset(); setChoice(next); }}
          search={searchCandidates} queryKey={["outlook-save-cv-candidates"]}
          placeholder="Name or email" noMatch="No matching candidates." />
      </Field>
      {context.isLoading && <p className="muted" role="status">Loading candidate…</p>}
      <p className="muted">People with access to this candidate’s CVs can see the saved file.</p>
      <ErrorMessage error={save.error} />
      <div className="row">
        <button type="submit" className="primary" disabled={!selected}>{save.isPending ? "Saving…" : "Save to CV & files"}</button>
        <button type="button" onClick={onClose}>Cancel</button>
      </div>
    </fieldset>
  </form>;
}
