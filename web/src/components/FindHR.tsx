// Company › Find HR: its people in HR on LinkedIn, read by the companion in
// the recruiter's own browser, a page at a time (country, job titles and
// pages chosen here). Who is a contact already, or may be, is shown; chosen
// people are added as contacts (their email guessed from colleagues', then
// verified like every new address). A click on a name opens the profile.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, ApiError, type Company, type Schemas, unwrap } from "../api/client";
import { companionAtLeast, companionCall, operationId, useCompanion } from "../companion";
import { linkedinDates } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { activeRuns, newRun, type Run, runScan, splitName, stopRun } from "./findHrRun";
import { companyPage } from "./linkedinCompany";

type Scan = Schemas["HrScan"];
type Person = Schemas["HrScanPerson"];
const COUNTRIES: [Schemas["HrCountry"], string][] = [["MY", "Malaysia"], ["SG", "Singapore"], ["ID", "Indonesia"], ["PH", "Philippines"],
  ["TH", "Thailand"], ["VN", "Vietnam"], ["HK", "Hong Kong"], ["TW", "Taiwan"], ["CN", "China"], ["JP", "Japan"], ["KR", "South Korea"],
  ["IN", "India"], ["AU", "Australia"], ["NZ", "New Zealand"], ["AE", "United Arab Emirates"], ["GB", "United Kingdom"], ["US", "United States"]];
const DEFAULT_KEYWORDS = "talent acquisition, recruiter, recruitment, human resources, HR, people and culture";
const NEEDS = "1.2.0";
const newer = (v: string | null) => companionAtLeast(v, NEEDS);

export const hrScanKey = (companyId: string) => ["hr-scan", companyId] as const;

export function FindHR({ company, canCreate }: { company: Company; canCreate: boolean }) {
  const queryClient = useQueryClient();
  const companion = useCompanion();
  const key = hrScanKey(company.id);
  const scan = useQuery({ queryKey: key, queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/hr-scan", { params: { path: { companyId: company.id } } })).scan });
  const [form, setForm] = useState({ country: "MY", keywords: DEFAULT_KEYWORDS, pages: 3 });
  const [editing, setEditing] = useState(false);
  // A run goes on while another tab of the company is shown; shown again here, it is picked up.
  const [running, setRunning] = useState<Run | null>(() => activeRuns.get(company.id) ?? null);
  const [note, setNote] = useState(() => activeRuns.get(company.id)?.note ?? "");
  const [error, setError] = useState<unknown>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [showHidden, setShowHidden] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const [datesAt, setDatesAt] = useState<{ done: number; of: number } | null>(null);
  useEffect(() => {
    if (!running) return;
    const t = setInterval(() => {
      setNow(Date.now());
      if (!activeRuns.has(company.id)) setRunning(null); // it ended while this tab was not shown
      else setNote(running.note);
    }, 1000);
    return () => { clearInterval(t); };
  }, [running, company.id]);

  const s = scan.data ?? null;
  const page = companyPage(company.linkedinUrl ?? "");
  const show = (next: Scan) => { queryClient.setQueryData(key, next); };

  const go = async (from: Scan) => {
    const run = newRun(setNote);
    activeRuns.set(company.id, run);
    setRunning(run); setError(null); setNow(run.started);
    try { await runScan(from, run, show); } catch (e) { setError(e); } finally { activeRuns.delete(company.id); setRunning(null); setNote(""); }
  };
  const start = async () => {
    setError(null);
    try {
      const id = await companionCall<{ company_id: string; company_name: string }>("companyId", { operation_id: operationId(), company_url: page });
      const fresh = unwrap(await api.PUT("/v1/companies/{companyId}/hr-scan", { params: { path: { companyId: company.id } }, body: {
        linkedinCompanyId: id.company_id, linkedinCompanyName: id.company_name || company.name, pages: form.pages,
        keywords: form.keywords.split(",").map((k) => k.trim()).filter(Boolean),
        ...(form.country ? { country: form.country as Schemas["HrCountry"] } : {}),
      } }));
      show(fresh); setEditing(false); setPicked([]);
      await go(fresh);
    } catch (e) { setError(e); }
  };

  const people = s?.people ?? [];
  const visible = people.filter((p) => showHidden || !p.hidden);
  const choosable = visible.filter((p) => !p.contactId);
  const chosen = people.filter((p) => picked.includes(p.url));
  const toggle = (p: Person) => { if (!p.contactId) setPicked((x) => (x.includes(p.url) ? x.filter((u) => u !== p.url) : [...x, p.url])); };

  // Read already: their dates (or that they left) are known. Clicking again after a
  // failure carries on from the person it stopped at, not from the first.
  const datesRead = (p: Person) => p.joined !== null || p.left !== null || p.current !== null;
  const dates = useMutation({
    mutationFn: async () => {
      if (!s) return;
      const todo = chosen.filter((p) => !datesRead(p));
      setDatesAt({ done: 0, of: todo.length });
      for (const [i, p] of todo.entries()) {
        const r = await companionCall<{ joined: string; left: string; current: boolean | null }>("employment",
          { operation_id: operationId(), url: p.url, company_url: page, company_id: s.linkedinCompanyId });
        show(unwrap(await api.POST("/v1/companies/{companyId}/hr-scan/joined", { params: { path: { companyId: company.id } },
          body: { url: p.url, current: r.current, joined: r.joined || null, left: r.left || null } })));
        setDatesAt({ done: i + 1, of: todo.length });
      }
    },
    onSettled: () => { setDatesAt(null); },
  });
  const add = useMutation({
    mutationFn: async () => {
      for (const p of chosen.filter((x) => !x.contactId)) {
        // Found by Find HR, so HR: tagged with the hr role (unless the agency removed it from its roles).
        const body = { companyId: company.id, ...splitName(p.name), title: p.title.slice(0, 200) || null, linkedinUrl: p.url };
        const c = await api.POST("/v1/contacts", { body: { ...body, role: "hr" } }).then(unwrap).catch((e: unknown) => {
          if (e instanceof ApiError && e.code === "VALIDATION_FAILED") return api.POST("/v1/contacts", { body }).then(unwrap);
          throw e;
        });
        const guess = unwrap(await api.GET("/v1/contacts/{contactId}/email-suggestion", { params: { path: { contactId: c.id } } })).suggestion;
        if (guess) await api.PATCH("/v1/contacts/{contactId}", { params: { path: { contactId: c.id } }, body: { version: c.version, email: guess.email } });
      }
    },
    onSuccess: async () => {
      setPicked([]);
      await queryClient.invalidateQueries({ queryKey: key });
      await queryClient.invalidateQueries({ queryKey: ["contacts"] });
    },
  });

  if (!page) return <section className="panel"><h3>Find HR</h3><p className="muted">Add the company&apos;s LinkedIn page (Edit company) to find its HR people.</p></section>;
  const settings = !s || editing;
  const seconds = running ? Math.max(0, Math.round((now - running.started) / 1000)) : 0;
  return (
    <section className="panel find-hr" aria-label="Find HR">
      <div className="row spread">
        <h3>Find HR <a className="muted small" href={page} target="_blank" rel="noreferrer">on LinkedIn ↗</a></h3>
        {s && !settings && !running && canCreate && <button type="button" className="link" onClick={() => { setEditing(true); }}>New search</button>}
      </div>
      {companion.ready === false && <p className="warning">Install and connect the LinkedIn companion (Settings › LinkedIn companion) to search.</p>}
      {companion.ready && !newer(companion.version) && <p className="warning">Update the LinkedIn companion to {NEEDS} to search.</p>}
      {settings && canCreate && (
        <form className="hr-settings" onSubmit={(e) => { e.preventDefault(); void start(); }}>
          <label className="field">Country
            <select value={form.country} onChange={(e) => { setForm({ ...form, country: e.target.value }); }}>
              <option value="">Anywhere</option>
              {COUNTRIES.map(([code, name]) => <option key={code} value={code}>{name}</option>)}
            </select>
          </label>
          <label className="field grow">Job titles (comma between)
            <input value={form.keywords} maxLength={600} onChange={(e) => { setForm({ ...form, keywords: e.target.value }); }} />
          </label>
          <label className="field">Pages per search
            <select value={form.pages} onChange={(e) => { setForm({ ...form, pages: Number(e.target.value) }); }}>
              {[1, 2, 3, 5, 10].map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
          </label>
          <div className="row">
            <button type="submit" disabled={!companion.ready || !newer(companion.version) || !!running}>Find HR people</button>
            {s && <button type="button" className="link" onClick={() => { setEditing(false); }}>Cancel</button>}
          </div>
        </form>
      )}
      {s && !settings && (
        <div className={`hr-status ${running ? "running" : s.status}`} role="status">
          <span>
            {running
              ? <>Reading search {String((s.next?.query ?? 0) + 1)} of {String(s.queries.length)}, page {String(s.next?.page ?? 1)} · {people.length} found</>
              : s.status === "done" ? <>Done · {String(s.pagesRead)} pages read · {people.length} found</>
                : s.status === "stopped" ? <>Stopped at search {String((s.next?.query ?? 0) + 1)}, page {String(s.next?.page ?? 1)}: {s.error} · {people.length} kept</>
                  : <>Paused at search {String((s.next?.query ?? 0) + 1)}, page {String(s.next?.page ?? 1)} · {people.length} found</>}
            <span className="muted"> · {s.countryName ?? "Anywhere"} · {s.pages} {s.pages === 1 ? "page" : "pages"} per search</span>
          </span>
          {running && <span className="studio-timer">{seconds}s</span>}
          {running && <button type="button" className="doc-btn" onClick={() => { stopRun(running); }}>Stop</button>}
          {!running && s.status !== "done" && canCreate && <button type="button" className="doc-btn primary" disabled={!companion.ready || !newer(companion.version)} onClick={() => { void go(s); }}>Resume</button>}
        </div>
      )}
      {note && <p className="muted small" role="status">{note}</p>}
      <ErrorMessage error={error ?? scan.error ?? dates.error ?? add.error} />
      {s && people.length > 0 && <>
        <div className="choose-row">
          <span className="choose" role="group" aria-label="Choose">
            <span className="choose-label" aria-hidden="true">Choose</span>
            <span className="segmented">
              <button type="button" className={choosable.length > 0 && choosable.every((p) => picked.includes(p.url)) ? "on" : undefined}
                onClick={() => { setPicked(choosable.map((p) => p.url)); }}>All {String(choosable.length)}</button>
              <button type="button" className={picked.length === 0 ? "on" : undefined} onClick={() => { setPicked([]); }}>None</button>
            </span>
          </span>
          {picked.length > 0 && canCreate && <>
            <button type="button" disabled={dates.isPending || !!running || chosen.every(datesRead)} onClick={() => { dates.mutate(); }}>{dates.isPending ? (datesAt ? `Reading dates… ${String(datesAt.done)} of ${String(datesAt.of)}` : "Reading dates…")
              : `Joining dates (${String(chosen.filter((p) => !datesRead(p)).length)})`}</button>
            <button type="button" className="primary" disabled={add.isPending} onClick={() => { add.mutate(); }}>{add.isPending ? "Adding…" : `Add ${String(picked.length)} as contacts`}</button>
          </>}
        </div>
        <div className="table-scroll">
          <table className="hr-people">
            <thead><tr><th>Person</th><th>Fit</th><th>Joined</th><th>In HiringPlug</th></tr></thead>
            <tbody>
              {visible.map((p) => (
                <tr key={p.url} className={`${picked.includes(p.url) ? "picked" : ""}${p.contactId ? " have" : ""}`} aria-selected={picked.includes(p.url)}
                  onClick={() => { toggle(p); }}>
                  <td>
                    <a className="hr-name" href={p.url} target="_blank" rel="noreferrer" onClick={(e) => { e.stopPropagation(); }}>{p.name}</a>
                    <div className="muted small">{p.title}{p.location ? ` · ${p.location}` : ""}</div>
                  </td>
                  <td className={`hr-tier t${p.tier}`} title="A: recruitment and HR leads · B: HR managers and partners · C: other HR roles">{p.tier}</td>
                  <td className="nowrap">{p.joined ? linkedinDates(p.joined) : p.current === false ? <span className="muted">left {linkedinDates(p.left)}</span> : <span className="muted">—</span>}</td>
                  <td>
                    {p.contactId ? <a className="hr-chip have" href={`#/contacts/${p.contactId}`} onClick={(e) => { e.stopPropagation(); }}>Contact</a>
                      : p.duplicateOf ? <a className="hr-chip dup" href={`#/contacts/${p.duplicateOf}`} onClick={(e) => { e.stopPropagation(); }}>Possible duplicate: {p.duplicateName}</a>
                        : <span className="hr-chip new">New</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {people.some((p) => p.hidden) && (
          <button type="button" className="link small" onClick={() => { setShowHidden(!showHidden); }}>
            {showHidden ? "Hide former staff and interns" : `${String(people.filter((p) => p.hidden).length)} hidden: former staff and interns · Show`}
          </button>
        )}
      </>}
      {s?.status === "done" && people.length === 0 && <p className="muted">LinkedIn showed no one with these job titles here.</p>}
    </section>
  );
}
