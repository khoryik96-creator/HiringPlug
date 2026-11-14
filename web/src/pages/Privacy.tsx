import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { privacyKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { formatDay, formatWhen, PRIVACY_KIND, PRIVACY_STATUS } from "../labels";
import { filledIn } from "../requiredFields";

type Request = Schemas["PrivacyRequest"];

/**
 * The data protection (PDPA) dashboard: requests to decide, consent counts,
 * and who asked not to be contacted. Nothing is deleted by the app: you
 * decide each request and record what you did.
 */
export function Privacy() {
  const overview = useQuery({ queryKey: privacyKey, queryFn: async () => unwrap(await api.GET("/v1/privacy")) });
  const o = overview.data;
  return (
    <main>
      <h2>Data protection (PDPA)</h2>
      <ErrorMessage error={overview.error} />
      {o && (
        <>
          <div className="stats">
            <div className="stat"><strong>{o.consent.given}</strong><span>consent given</span></div>
            <div className="stat"><strong>{o.consent.withdrawn}</strong><span>consent withdrawn (do not contact)</span></div>
            <div className="stat"><strong>{o.consent.notRecorded}</strong><span>no consent recorded</span></div>
            <div className="stat"><strong>{o.openRequests.length}</strong><span>open requests</span></div>
          </div>
          <section className="panel" aria-label="Open requests">
            <h3>Open requests</h3>
            {o.openRequests.length === 0 && <p className="muted">None.</p>}
            {o.openRequests.map((r) => <OpenRequest key={r.id} r={r} />)}
          </section>
          <section className="panel" aria-label="Do not contact">
            <h3>Withdrew consent ({o.withdrawn.length})</h3>
            {o.withdrawn.length === 0 ? <p className="muted">Nobody.</p> : (
              <ul>
                {o.withdrawn.map((w) => (
                  <li key={w.candidateId}><a href={`#/candidates/${w.candidateId}`}>{w.candidateName}</a> <span className="muted">since {formatDay(w.day)}</span></li>
                ))}
              </ul>
            )}
          </section>
          {o.closedRequests.length > 0 && (
            <section className="panel" aria-label="Recently closed">
              <h3>Recently closed</h3>
              <table>
                <thead><tr><th>Candidate</th><th>Request</th><th>Decision</th><th>What was done</th><th>Closed</th><th /></tr></thead>
                <tbody>
                  {o.closedRequests.map((r) => (
                    <tr key={r.id}>
                      <td><a href={`#/candidates/${r.candidateId}`}>{r.candidateName}</a></td>
                      <td>{PRIVACY_KIND[r.kind]}</td>
                      <td><span className={`badge ${r.status}`}>{PRIVACY_STATUS[r.status]}</span></td>
                      <td>{r.outcome}</td>
                      <td className="muted">{r.closedAt && formatWhen(r.closedAt)}{r.closedByName && ` by ${r.closedByName}`}</td>
                      <td><Reopen r={r} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          )}
        </>
      )}
    </main>
  );
}

function useDecide(r: Request) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: { status: Schemas["PrivacyRequestStatus"]; outcome?: string }) =>
      unwrap(await api.PATCH("/v1/privacy-requests/{requestId}", { params: { path: { requestId: r.id } }, body: { version: r.version, ...body } })),
    onSettled: () => queryClient.invalidateQueries({ queryKey: privacyKey }),
  });
}

function OpenRequest({ r }: { r: Request }) {
  const [outcome, setOutcome] = useState("");
  const decide = useDecide(r);
  const due = r.dueOn && (r.overdue ? `overdue since ${formatDay(r.dueOn)}` : `answer by ${formatDay(r.dueOn)}`);
  return (
    <article className="privacy-request" aria-label={`${PRIVACY_KIND[r.kind]}: ${r.candidateName}`}>
      <div>
        <a href={`#/candidates/${r.candidateId}`}>{r.candidateName}</a>: <strong>{PRIVACY_KIND[r.kind]}</strong>{" "}
        <span className="muted">received {formatDay(r.receivedOn)}, {r.daysOpen} day(s) ago</span>
        {due && <span className={r.overdue ? "warning" : "muted"}> · {due}</span>}
      </div>
      {r.details && <div className="muted small">{r.details}</div>}
      {r.kind === "withdraw_consent" && <div className="muted small">Done also marks them do not contact (emails to them are refused).</div>}
      {r.kind === "removal" && <div className="muted small">HiringPlug does not delete anything: say below what you did.</div>}
      <form className="row" onSubmit={(e) => { e.preventDefault(); decide.mutate({ status: "done", outcome }); }}>
        <input required aria-label="What was done" placeholder="Outcome" maxLength={2000} value={outcome}
          onChange={(e) => { setOutcome(e.target.value); }} />
        <button type="submit" disabled={decide.isPending}>Done</button>
        <button type="button" disabled={decide.isPending} onClick={(e) => { if (filledIn(e.currentTarget.form)) decide.mutate({ status: "declined", outcome }); }}>Decline</button>
      </form>
      <ErrorMessage error={decide.error} />
    </article>
  );
}

function Reopen({ r }: { r: Request }) {
  const decide = useDecide(r);
  return (
    <>
      <button type="button" className="link" disabled={decide.isPending} onClick={() => { decide.mutate({ status: "open" }); }}>Reopen</button>
      <ErrorMessage error={decide.error} />
    </>
  );
}
