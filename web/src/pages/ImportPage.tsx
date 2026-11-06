import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import type { Permission } from "../api/client";
import { CvImports } from "../components/CvImports";
import { parseCsv, toCsv } from "../import/csv";
import { autoMap, FIELDS, type ImportKind, mappingProblems } from "../import/mapping";
import { type RowResult, runImport } from "../import/runImport";

const KINDS: { kind: ImportKind; label: string; permission: Permission }[] = [
  { kind: "candidates", label: "Candidates", permission: "candidates:create" },
  { kind: "companies", label: "Companies", permission: "companies:create" },
  { kind: "contacts", label: "Contacts", permission: "contacts:create" },
];

const STATUS: Record<RowResult["status"], string> = { added: "Added", exists: "Already there", fix: "To fix", failed: "Failed" };

/** Import CVs and LinkedIn PDFs, or candidates, companies or contacts from a CSV file, with a report. */
export function ImportPage({ permissions }: { permissions: Permission[] }) {
  const cvs = permissions.includes("intake:create");
  return (
    <main>
      <h2>Import</h2>
      {cvs && <CvImports />}
      <CsvImport permissions={permissions} cvs={cvs} />
    </main>
  );
}

/** Candidates, companies or contacts from a CSV file. */
function CsvImport({ permissions, cvs }: { permissions: Permission[]; cvs: boolean }) {
  const queryClient = useQueryClient();
  const allowed = KINDS.filter((k) => permissions.includes(k.permission));
  const [kind, setKind] = useState<ImportKind>(allowed[0]?.kind ?? "candidates");
  const [file, setFile] = useState<{ name: string; header: string[]; rows: string[][] } | null>(null);
  const [mapping, setMapping] = useState<string[]>([]);
  const [createCompanies, setCreateCompanies] = useState(true);
  const [progress, setProgress] = useState<number | null>(null);
  const [results, setResults] = useState<RowResult[] | null>(null);

  if (allowed.length === 0) return cvs ? null : <p className="muted">Your role cannot add records.</p>;

  const choose = (k: ImportKind, f = file) => {
    setKind(k);
    setResults(null);
    if (f) setMapping(autoMap(k, f.header));
  };
  const load = async (picked: File) => {
    const [header = [], ...rows] = parseCsv(await picked.text());
    const f = { name: picked.name, header, rows };
    setFile(f);
    choose(kind, f);
  };
  const problems = file ? mappingProblems(kind, mapping) : [];
  const start = async () => {
    if (!file) return;
    setResults(null);
    setProgress(0);
    const r = await runImport(kind, file.rows, mapping, { createCompanies }, setProgress);
    setProgress(null);
    setResults(r);
    void queryClient.invalidateQueries();
  };
  const toFix = results?.filter((r) => r.status === "fix" || r.status === "failed") ?? [];
  const downloadToFix = () => {
    if (!file) return;
    const rows = [[...file.header, "Problem"], ...toFix.map((r) => [...(file.rows[r.line - 2] ?? []), r.message])];
    const url = URL.createObjectURL(new Blob([toCsv(rows)], { type: "text/csv" }));
    const a = Object.assign(document.createElement("a"), { href: url, download: `to-fix-${file.name}` });
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <>
      <section className="panel" aria-label="CSV import">
        <h3>From a spreadsheet</h3>
        <p className="muted small">CSV, with column names in the first row.</p>
        <div className="row">
          <label className="row">What: <select value={kind} onChange={(e) => { choose(e.target.value as ImportKind); }}>
            {allowed.map((k) => <option key={k.kind} value={k.kind}>{k.label}</option>)}
          </select></label>
          <input type="file" accept=".csv,text/csv" aria-label="CSV file" onChange={(e) => { const f = e.target.files?.[0]; if (f) void load(f); }} />
        </div>
      </section>

      {file && (
        <section className="panel" aria-label="Columns">
          <h3>{file.name}: {file.rows.length} rows. Which column is what?</h3>
          <table>
            <thead><tr><th>Column in the file</th><th>Imported as</th><th>First rows</th></tr></thead>
            <tbody>
              {file.header.map((h, i) => (
                <tr key={`${h}-${String(i)}`}>
                  <td>{h}</td>
                  <td>
                    <select aria-label={`Column ${h}`} value={mapping[i] ?? ""} onChange={(e) => { setMapping((m) => m.map((v, j) => (j === i ? e.target.value : v))); }}>
                      <option value="">— not imported —</option>
                      {FIELDS[kind].map((f) => <option key={f.key} value={f.key}>{f.label}</option>)}
                    </select>
                  </td>
                  <td className="muted">{file.rows.slice(0, 3).map((r) => r[i]).filter(Boolean).join(" · ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {kind === "contacts" && (
            <label className="row"><input type="checkbox" checked={createCompanies} onChange={(e) => { setCreateCompanies(e.target.checked); }} /> Add companies that do not exist yet</label>
          )}
          {problems.map((p) => <p key={p} className="error">{p}</p>)}
          <div className="row">
            <button type="button" disabled={problems.length > 0 || progress !== null || file.rows.length === 0} onClick={() => { void start(); }}>
              Import {file.rows.length} {kind}
            </button>
            {progress !== null && <span className="muted">{progress} of {file.rows.length}…</span>}
          </div>
        </section>
      )}

      {results && (
        <section className="panel" aria-label="Import report">
          <h3>
            {results.filter((r) => r.status === "added").length} added · {results.filter((r) => r.status === "exists").length} already there · {toFix.length} to fix
          </h3>
          {toFix.length > 0 && <button type="button" onClick={downloadToFix}>Download the rows to fix (CSV)</button>}
          <table>
            <thead><tr><th>Line</th><th>Name</th><th>Result</th><th>Details</th></tr></thead>
            <tbody>
              {results.map((r) => (
                <tr key={r.line}>
                  <td>{r.line}</td>
                  <td>{r.link ? <a href={r.link}>{r.label}</a> : r.label}</td>
                  <td><span className={`badge ${r.status === "added" ? "valid" : r.status === "exists" ? "unknown" : "invalid"}`}>{STATUS[r.status]}</span></td>
                  <td className="muted">{r.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </>
  );
}
