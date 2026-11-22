// Making a client shortlist link: the chosen candidates, each with one of
// their formatted CVs (blind or with the name), an optional note, how long
// the link works, and whether to email it to the job's hiring contact.
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Document, type Schemas, unwrap } from "../../api/client";
import { STAGE } from "../../labels";
import { CopyButton } from "../CopyButton";
import { ErrorMessage } from "../ErrorMessage";
import { clientShortlistsKey } from "../../api/keys";

/** A candidate on the job, to put on a link. */
export type LinkCandidate = { applicationId: string; candidateId: string; name: string; stage: Schemas["Stage"] };

export const DAYS = [7, 14, 30, 60, 90] as const;

/** The CVs that may go to a client: the formatted ones, blind first, newest first. */
export function clientCvs(docs: Document[]): Document[] {
  const blind = (d: Document) => (d.fileName.includes("(Blinded)") ? 0 : 1);
  return docs.filter((d) => d.source === "formatted")
    .sort((a, b) => blind(a) - blind(b) || b.createdAt.localeCompare(a.createdAt));
}

export function ClientLinkForm({ jobId, people: chosen, contactName, canEmail, onClose }: {
  jobId: string; people: LinkCandidate[]; contactName?: string | null; canEmail: boolean; onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [people, setPeople] = useState(chosen);
  const [cvOf, setCvOf] = useState<Record<string, string>>({});
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const [days, setDays] = useState(14);
  const [email, setEmail] = useState(false);
  const docs = useQueries({
    queries: people.map((p) => ({
      queryKey: ["documents", p.candidateId],
      queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId: p.candidateId } } })).items,
    })),
  });
  const cvsAt = (i: number) => clientCvs(docs[i]?.data ?? []);
  const chosenCv = (p: LinkCandidate, i: number) => cvOf[p.applicationId] ?? cvsAt(i)[0]?.id ?? "";
  const missing = people.filter((p, i) => docs[i]?.data && !chosenCv(p, i));
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/jobs/{jobId}/client-shortlists", {
      params: { path: { jobId } },
      body: {
        candidates: people.map((p, i) => ({
          applicationId: p.applicationId, documentId: chosenCv(p, i), ...(notes[p.applicationId]?.trim() ? { note: notes[p.applicationId] } : {}),
        })),
        expiresInDays: days, emailHiringContact: email, ...(message.trim() ? { message } : {}),
      },
    })),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: clientShortlistsKey(jobId) }); },
  });

  if (create.data) {
    const made = create.data;
    return (
      <section className="panel client-link" aria-label="Client link made">
        <h3>Link ready for the client</h3>
        <p>Copy it now and send it to the client: for safety it is shown only this once.</p>
        <div className="client-link-url">
          <input readOnly value={made.url} aria-label="Client link" onFocus={(e) => { e.target.select(); }} />
          <CopyButton text={made.url} what="client link" />
        </div>
        {made.shortlist.emailedTo && <p className="muted small">Also emailed to {made.shortlist.emailedTo}.</p>}
        <p className="muted small">It works until {new Date(made.shortlist.expiresAt).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" })}. The client&apos;s answers appear under Client links, and move each candidate on the pipeline.</p>
        <button type="button" onClick={onClose}>Done</button>
      </section>
    );
  }
  return (
    <section className="panel client-link" aria-label="New client link">
      <div className="row spread">
        <h3>Client link for {people.length} candidate{people.length === 1 ? "" : "s"}</h3>
        <button type="button" onClick={onClose}>Cancel</button>
      </div>
      <p className="muted small">
        The client sees only these candidates, each with the CV you choose (a blind CV shows a reference instead of the name), never their salary,
        email or phone. They approve or reject each one; approved candidates move to Interview, rejected ones to Rejected (client said no).
      </p>
      {people.length === 0 && <p className="warning">Choose candidates on the pipeline first.</p>}
      <table className="client-link-table">
        <thead><tr><th>Candidate</th><th>CV the client sees</th><th>Your note for the client <span className="muted">(optional)</span></th><th /></tr></thead>
        <tbody>
          {people.map((p, i) => {
            const cvs = cvsAt(i);
            return (
              <tr key={p.applicationId}>
                <td><a href={`#/candidates/${p.candidateId}`}>{p.name}</a> <span className={`badge ${p.stage}`}>{STAGE[p.stage]}</span></td>
                <td>
                  {docs[i]?.isPending ? <span className="muted small">Loading CVs…</span> : cvs.length === 0 ? (
                    <span className="warning small">No formatted CV yet: make one (blind or formatted) on <a href={`#/candidates/${p.candidateId}`}>their page</a>.</span>
                  ) : (
                    <select aria-label={`CV for ${p.name}`} value={chosenCv(p, i)} onChange={(e) => { setCvOf({ ...cvOf, [p.applicationId]: e.target.value }); }}>
                      {cvs.map((d) => <option key={d.id} value={d.id}>{d.fileName}</option>)}
                    </select>
                  )}
                </td>
                <td><input aria-label={`Note for ${p.name}`} maxLength={1000} value={notes[p.applicationId] ?? ""}
                  onChange={(e) => { setNotes({ ...notes, [p.applicationId]: e.target.value }); }} /></td>
                <td>{people.length > 1 && <button type="button" className="link" aria-label={`Leave out ${p.name}`}
                  onClick={() => { setPeople(people.filter((x) => x !== p)); }}>×</button>}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      <label className="field">Message to the client <span className="muted small">(optional, at the top of their page)</span>
        <textarea rows={2} maxLength={2000} value={message} onChange={(e) => { setMessage(e.target.value); }} />
      </label>
      <div className="row client-link-options">
        <label className="row">Link works for
          <select value={days} onChange={(e) => { setDays(Number(e.target.value)); }}>
            {DAYS.map((d) => <option key={d} value={d}>{d} days</option>)}
          </select>
        </label>
        {canEmail && (
          <label className="row check">
            <input type="checkbox" checked={email} disabled={!contactName} onChange={(e) => { setEmail(e.target.checked); }} />
            {contactName ? `Also email it to ${contactName} (hiring contact)` : "Email it to the hiring contact (the job has none)"}
          </label>
        )}
      </div>
      <ErrorMessage error={create.error} />
      <div className="row">
        <button type="button" className="primary" disabled={create.isPending || people.length === 0 || missing.length > 0 || docs.some((d) => d.isPending)}
          onClick={() => { create.mutate(); }}>{email ? "Make the link and email it" : "Make the link"}</button>
        {missing.length > 0 && <span className="warning small">Leave out {missing.map((p) => p.name).join(", ")} or make their CV first.</span>}
      </div>
    </section>
  );
}
