// The client's shortlist page (no sign-in): the candidates a recruiter chose,
// each with their CV, to approve or reject with a comment. Lives at
// /shortlist/<token>.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";

type Page = Schemas["PublicClientShortlist"];
type Item = Schemas["PublicShortlistCandidate"];

const ROUTE = /^\/shortlist\/([A-Za-z0-9_-]{43})\/?$/;

/** The token of a shortlist page address (/shortlist/<token>, or #/shortlist/<token>). */
export function shortlistRoute(loc: Pick<Location, "pathname" | "hash">): string | null {
  return ROUTE.exec(loc.hash.replace(/^#/, ""))?.[1] ?? ROUTE.exec(loc.pathname)?.[1] ?? null;
}

const day = (iso: string) => new Date(iso).toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric" });

/** A CV as the client gets it: its bytes, the name to save it as, and whether it is a PDF. */
export type CvCopy = { url: string; fileName: string; pdf: boolean };

/** Fetches a candidate's CV (named by the label, never the candidate). */
export async function fetchCv(token: string, item: Item): Promise<CvCopy> {
  const res = await fetch(`/v1/public/client-shortlists/${token}/candidates/${item.id}/cv`);
  if (!res.ok) throw new Error(res.status === 409 ? "This link no longer works." : "The CV could not be opened. Try again in a moment.");
  const named = /filename\*=utf-8''([^;]+)|filename="?([^";]+)"?/i.exec(res.headers.get("Content-Disposition") ?? "");
  const fileName = named ? decodeURIComponent(named[1] ?? named[2] ?? "") : (item.cvFileName ?? `${item.label} CV.pdf`);
  const pdf = fileName.toLowerCase().endsWith(".pdf");
  const blob = new Blob([await res.arrayBuffer()], { type: pdf ? "application/pdf" : "application/octet-stream" });
  return { url: URL.createObjectURL(blob), fileName, pdf };
}

function save(cv: CvCopy) {
  const a = document.createElement("a");
  a.href = cv.url;
  a.download = cv.fileName;
  a.click();
}

export function ClientShortlistPage({ token }: { token: string }) {
  const queryClient = useQueryClient();
  const key = ["public-shortlist", token];
  const page = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/public/client-shortlists/{shortlistToken}", { params: { path: { shortlistToken: token } } })),
  });
  const answer = useMutation({
    mutationFn: async (a: { id: string; decision: Schemas["ClientShortlistDecision"]; comment: string }) =>
      unwrap(await api.POST("/v1/public/client-shortlists/{shortlistToken}/candidates/{shortlistCandidateId}/answer", {
        params: { path: { shortlistToken: token, shortlistCandidateId: a.id } },
        body: { decision: a.decision, ...(a.comment.trim() ? { comment: a.comment } : {}) },
      })),
    onSuccess: (p) => { queryClient.setQueryData(key, p); },
    onError: async () => { await queryClient.invalidateQueries({ queryKey: key }); },
  });
  const p = page.data;
  return (
    <div className="booking-page shortlist-page">
      <main className="booking-card shortlist-card">
        {!p ? (page.error ? <ErrorMessage error={page.error} /> : <p className="muted">Loading…</p>) : (
          <Shortlist p={p} token={token} busy={answer.isPending} error={answer.error}
            onAnswer={(id, decision, comment) => { answer.mutate({ id, decision, comment }); }} />
        )}
      </main>
    </div>
  );
}

function Shortlist({ p, token, busy, error, onAnswer }: {
  p: Page; token: string; busy: boolean; error: Error | null;
  onAnswer: (id: string, decision: Schemas["ClientShortlistDecision"], comment: string) => void;
}) {
  const who = p.recruiterName ?? p.agencyName;
  const answered = p.candidates.filter((c) => c.decision).length;
  return (
    <>
      <p className="booking-company">{p.agencyName}</p>
      <h1>Shortlist: {p.jobTitle}</h1>
      {p.status === "open" && (
        <p className="muted">
          From {who} · {p.candidates.length} candidate{p.candidates.length === 1 ? "" : "s"} · open until {day(p.expiresAt)}
        </p>
      )}
      {p.status === "revoked" && <p className="booking-note">This link was withdrawn. Ask {who} for a new one if you still need it.</p>}
      {p.status === "expired" && <p className="booking-note">This link has expired. Ask {who} for a new one.</p>}
      {p.status === "open" && (
        <>
          {p.message && <blockquote className="shortlist-message">{p.message}</blockquote>}
          <p className="shortlist-howto">
            Open each CV, then approve the candidates you would like to interview or reject the others. A comment helps {who} (optional). Each answer goes to {who} at once.
            {" "}<span className="muted">{answered} of {p.candidates.length} answered.</span>
          </p>
          <ErrorMessage error={error} />
          <ol className="shortlist-list">
            {p.candidates.map((c, i) => <Candidate key={c.id} c={c} n={i + 1} token={token} busy={busy} who={who} onAnswer={onAnswer} />)}
          </ol>
          {p.candidates.length === 0 && <p className="booking-note">There is no one on this shortlist any more.</p>}
        </>
      )}
    </>
  );
}

function Candidate({ c, n, token, busy, who, onAnswer }: {
  c: Item; n: number; token: string; busy: boolean; who: string;
  onAnswer: (id: string, decision: Schemas["ClientShortlistDecision"], comment: string) => void;
}) {
  const [comment, setComment] = useState("");
  const [shown, setShown] = useState(false);
  // A PDF shows on the page; a Word file downloads.
  const cv = useMutation({
    mutationFn: () => fetchCv(token, c),
    onSuccess: (copy) => { if (copy.pdf) setShown(true); else save(copy); },
  });
  useEffect(() => () => { if (cv.data) URL.revokeObjectURL(cv.data.url); }, [cv.data]);
  const copy = shown ? cv.data : undefined;
  return (
    <li className={`shortlist-item${c.decision ? ` answered ${c.decision}` : ""}`} aria-label={c.label}>
      <div className="shortlist-item-head">
        <span className="shortlist-n" aria-hidden="true">{n}</span>
        <h2>{c.label}</h2>
        {c.blind && <span className="chip small" title="Names and contact details are left out">Blind CV</span>}
        <span className="shortlist-cv">
          {copy ? (
            <>
              <button type="button" onClick={() => { save(copy); }}>Download</button>
              <button type="button" onClick={() => { setShown(false); }}>Hide CV</button>
            </>
          ) : (
            <button type="button" disabled={cv.isPending} onClick={() => { if (cv.data?.pdf) setShown(true); else cv.mutate(); }}>
              {cv.isPending ? "Opening…" : "View CV"}
            </button>
          )}
        </span>
      </div>
      {copy && <iframe className="shortlist-frame" title={`CV of ${c.label}`} src={`${copy.url}#navpanes=0&view=FitH`} />}
      {c.note && <p className="shortlist-note"><span className="muted small">{who}: </span>{c.note}</p>}
      <ErrorMessage error={cv.error} />
      {c.decision ? (
        <p className={`shortlist-answer ${c.decision}`} role="status">
          <strong>{c.decision === "approved" ? "✓ Approved" : "✕ Rejected"}</strong>
          {c.answeredAt && <span className="muted small"> · {day(c.answeredAt)}</span>}
          {c.comment && <span className="shortlist-comment">“{c.comment}”</span>}
        </p>
      ) : (
        <div className="shortlist-decide">
          <label className="field">Comment <span className="muted small">(optional)</span>
            <textarea rows={2} maxLength={2000} value={comment} placeholder="e.g. Strong on reporting; can they start in March?"
              onChange={(e) => { setComment(e.target.value); }} />
          </label>
          <div className="row">
            <button type="button" className="primary approve" disabled={busy} onClick={() => { onAnswer(c.id, "approved", comment); }}>Approve for interview</button>
            <button type="button" className="reject" disabled={busy} onClick={() => { onAnswer(c.id, "rejected", comment); }}>Reject</button>
          </div>
        </div>
      )}
    </li>
  );
}
