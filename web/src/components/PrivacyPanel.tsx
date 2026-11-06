import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { candidatePrivacyKey, privacyKey } from "../api/keys";
import { CONSENT_METHOD, CONSENT_STATUS, formatDay, keysOf, PRIVACY_KIND, PRIVACY_STATUS } from "../labels";
import { exportText, saveText } from "./dataExport";
import { ErrorMessage } from "./ErrorMessage";

type Consent = Schemas["Consent"];

/** Today in the browser's time zone, as YYYY-MM-DD. */
export function todayISO(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(now.getFullYear())}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

/** A consent record in words, e.g. "Given on 3 Nov 2026 (Email)". */
export function consentText(c: Consent | null): string {
  if (!c) return "No consent recorded";
  return `${CONSENT_STATUS[c.status]} on ${formatDay(c.day)} (${CONSENT_METHOD[c.method]})`;
}

/**
 * A candidate's data protection (PDPA): their consent, the requests they made
 * about their data, and a copy of what is held about them. Nothing here
 * deletes anything.
 */
export function PrivacyPanel({ candidateId, candidateName, permissions }: { candidateId: string; candidateName: string; permissions: Permission[] }) {
  const key = candidatePrivacyKey(candidateId);
  const data = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/privacy", { params: { path: { candidateId } } })),
    enabled: permissions.includes("privacy:read"),
  });
  const [editing, setEditing] = useState<"consent" | "request" | null>(null);
  if (!permissions.includes("privacy:read")) return null;
  const p = data.data;
  const canRecord = permissions.includes("privacy:record");
  return (
    <section className="panel" aria-label="Data protection">
      <h3>Data protection</h3>
      <ErrorMessage error={data.error} />
      {p && (
        <>
          <p className={p.doNotContact ? "warning" : undefined}>
            <strong>Consent:</strong> {consentText(p.consent)}
            {p.doNotContact && " · Do not contact: emails to them are refused."}
            {p.consent?.note && <span className="muted small"> · {p.consent.note}</span>}
          </p>
          {p.requests.length > 0 && (
            <ul className="privacy-requests">
              {p.requests.map((r) => (
                <li key={r.id}>
                  <strong>{PRIVACY_KIND[r.kind]}</strong> · received {formatDay(r.receivedOn)} ·{" "}
                  <span className={`badge ${r.status}`}>{PRIVACY_STATUS[r.status]}</span>
                  {r.dueOn && r.status === "open" && (
                    <span className={r.overdue ? "warning" : "muted"}> · {r.overdue ? "overdue since" : "answer by"} {formatDay(r.dueOn)}</span>
                  )}
                  {r.details && <div className="muted small">{r.details}</div>}
                  {r.outcome && <div className="small">Outcome: {r.outcome}</div>}
                </li>
              ))}
            </ul>
          )}
          {permissions.includes("privacy:manage") && p.requests.some((r) => r.status === "open") && (
            <p className="muted small">Decide open requests on the <a href="#/privacy">Data protection</a> page.</p>
          )}
          <div className="row">
            {canRecord && editing === null && (
              <>
                <button type="button" onClick={() => { setEditing("consent"); }}>Record consent</button>
                <button type="button" onClick={() => { setEditing("request"); }}>Log a request</button>
              </>
            )}
            {permissions.includes("privacy:manage") && <DownloadCopy candidateId={candidateId} candidateName={candidateName} />}
          </div>
          {editing === "consent" && <ConsentForm candidateId={candidateId} consent={p.consent} onDone={() => { setEditing(null); }} />}
          {editing === "request" && <RequestForm candidateId={candidateId} onDone={() => { setEditing(null); }} />}
        </>
      )}
    </section>
  );
}

function ConsentForm({ candidateId, consent, onDone }: { candidateId: string; consent: Consent | null; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<Schemas["ConsentStatus"]>(consent?.status === "given" ? "withdrawn" : "given");
  const [method, setMethod] = useState<Schemas["ConsentMethod"]>("email");
  const [day, setDay] = useState(todayISO());
  const [note, setNote] = useState("");
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/candidates/{candidateId}/consent", {
      params: { path: { candidateId } },
      body: { expectedVersion: consent?.version ?? 0, status, method, day, note: note.trim() || null },
    })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: privacyKey });
      onDone();
    },
  });
  return (
    <form className="row" aria-label="Record consent" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <select aria-label="Consent" value={status} onChange={(e) => { setStatus(e.target.value as typeof status); }}>
        {keysOf(CONSENT_STATUS).map((s) => <option key={s} value={s}>{CONSENT_STATUS[s]}</option>)}
      </select>
      <select aria-label="How" value={method} onChange={(e) => { setMethod(e.target.value as typeof method); }}>
        {keysOf(CONSENT_METHOD).filter((m) => m !== "request").map((m) => <option key={m} value={m}>{CONSENT_METHOD[m]}</option>)}
      </select>
      <input aria-label="On" type="date" value={day} max={todayISO()} onChange={(e) => { setDay(e.target.value); }} />
      <input aria-label="Note (optional)" placeholder="Note (optional)" maxLength={1000} value={note} onChange={(e) => { setNote(e.target.value); }} />
      <button type="submit" disabled={save.isPending || !day}>Save</button>
      <button type="button" className="link" onClick={onDone}>Cancel</button>
      <ErrorMessage error={save.error} />
    </form>
  );
}

function RequestForm({ candidateId, onDone }: { candidateId: string; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<Schemas["PrivacyRequestKind"]>("access");
  const [receivedOn, setReceivedOn] = useState(todayISO());
  const [details, setDetails] = useState("");
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/candidates/{candidateId}/privacy-requests", {
      params: { path: { candidateId } }, body: { kind, receivedOn, details: details.trim() || null },
    })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: privacyKey });
      onDone();
    },
  });
  return (
    <form className="row" aria-label="Log a request" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <select aria-label="Request" value={kind} onChange={(e) => { setKind(e.target.value as typeof kind); }}>
        {keysOf(PRIVACY_KIND).map((k) => <option key={k} value={k}>{PRIVACY_KIND[k]}</option>)}
      </select>
      <input aria-label="Received on" type="date" value={receivedOn} max={todayISO()} onChange={(e) => { setReceivedOn(e.target.value); }} />
      <input aria-label="Details (optional)" placeholder="Details (optional)" maxLength={2000} value={details} onChange={(e) => { setDetails(e.target.value); }} />
      <button type="submit" disabled={save.isPending || !receivedOn}>Log request</button>
      <button type="button" className="link" onClick={onDone}>Cancel</button>
      <ErrorMessage error={save.error} />
    </form>
  );
}

/** Downloads a copy of everything held about the candidate, as text or JSON. */
function DownloadCopy({ candidateId, candidateName }: { candidateId: string; candidateName: string }) {
  const get = useMutation({
    mutationFn: async (as: "text" | "json") => {
      const e = unwrap(await api.GET("/v1/candidates/{candidateId}/data-export", { params: { path: { candidateId } } }));
      const name = `${candidateName} - personal data ${e.exportedAt.slice(0, 10)}`;
      if (as === "text") saveText(exportText(e), `${name}.txt`, "text/plain;charset=utf-8");
      else saveText(JSON.stringify(e, null, 2), `${name}.json`, "application/json");
    },
  });
  return (
    <span>
      <button type="button" className="link" disabled={get.isPending} onClick={() => { get.mutate("text"); }}>Download a copy of their data</button>{" "}
      <button type="button" className="link small" disabled={get.isPending} onClick={() => { get.mutate("json"); }}>(JSON)</button>
      <ErrorMessage error={get.error} />
    </span>
  );
}
