import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import type { Permission } from "../api/client";
import { CvImports } from "../components/CvImports";
import { OutlookCvImport } from "../components/outlook/OutlookCvImport";
import { hashParam } from "../hashQuery";
import { parseCsv, rowsToFix, toCsv, toFixName } from "../import/csv";
import { readXlsx } from "../import/xlsx";
import { autoMap, FIELDS, type ImportKind, mappingProblems } from "../import/mapping";
import { type RowResult, runImport } from "../import/runImport";

const KINDS: { kind: ImportKind; label: string; permission: Permission }[] = [
  { kind: "candidates", label: "Candidates", permission: "candidates:create" },
  { kind: "companies", label: "Companies", permission: "companies:create" },
  { kind: "contacts", label: "Contacts", permission: "contacts:create" },
];

const STATUS: Record<RowResult["status"], string> = { added: "Added", exists: "Already there", fix: "To fix", failed: "Failed" };

/** Everything that comes in from files: CVs and LinkedIn PDFs (also on the dashboard), and spreadsheets. */
export function ImportPage({ permissions }: { permissions: Permission[] }) {
  const cvs = permissions.includes("intake:create");
  const messageId = hashParam("outlookMessage");
  const attachmentId = hashParam("outlookAttachment");
  return (
    <main>
      <h2>Import</h2>
      {cvs && (permissions.includes("emails:send") && messageId && attachmentId
        ? <OutlookCvImport key={JSON.stringify([messageId, attachmentId])} messageId={messageId} attachmentId={attachmentId} />
        : <CvImports />)}
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
  const [report, setReport] = useState<{ source: NonNullable<typeof file>; results: RowResult[] } | null>(null);
  const [error, setError] = useState("");
  const reading = useRef(0);
  const kindRef = useRef(kind);
  const results = report?.results;

  if (allowed.length === 0) return cvs ? null : <p className="muted">Your role cannot add records.</p>;

  const choose = (k: ImportKind, f = file) => {
    setKind(k);
    kindRef.current = k;
    setReport(null);
    setError("");
    if (f) setMapping(autoMap(k, f.header));
  };
  const load = async (picked: File) => {
    const id = ++reading.current;
    setFile(null);
    setReport(null);
    setError("");
    try {
      const name = picked.name.toLowerCase();
      if (name.endsWith(".xls")) {
        setError("That is an old Excel file (.xls). Open it in Excel and save it as .xlsx or CSV, then choose it again.");
        return;
      }
      const [header = [], ...rows] = name.endsWith(".xlsx") ? await readXlsx(await picked.arrayBuffer()) : parseCsv(await picked.text());
      if (reading.current !== id) return;
      const f = { name: picked.name, header, rows };
      setFile(f);
      choose(kindRef.current, f);
    } catch {
      if (reading.current === id) setError(`Could not read ${picked.name}. Save it as .xlsx or CSV and try again.`);
    }
  };
  const problems = file ? mappingProblems(kind, mapping) : [];
  const start = async () => {
    if (!file) return;
    setReport(null);
    setError("");
    setProgress(0);
    try {
      const rows = await runImport(kind, file.rows, mapping, { createCompanies }, setProgress);
      setReport({ source: file, results: rows });
    } catch {
      setError("Import stopped. Check the records already added before retrying.");
    } finally {
      setProgress(null);
      void queryClient.invalidateQueries();
    }
  };
  const toFix = results?.filter((r) => r.status === "fix" || r.status === "failed") ?? [];
  const downloadToFix = () => {
    if (!report) return;
    const source = report.source;
    const rows = rowsToFix(source.header, source.rows, toFix);
    const url = URL.createObjectURL(new Blob([toCsv(rows)], { type: "text/csv" }));
    const a = Object.assign(document.createElement("a"), { href: url, download: toFixName(source.name) });
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <>
      <section className="panel" aria-label="Spreadsheet import">
        <h3>From a spreadsheet</h3>
        <p className="muted small">Excel (.xlsx) or CSV, with column names in the first row (from Excel, its first sheet).</p>
        <div className="row">
          <label className="row">What: <select disabled={progress !== null} value={kind} onChange={(e) => { choose(e.target.value as ImportKind); }}>
            {allowed.map((k) => <option key={k.kind} value={k.kind}>{k.label}</option>)}
          </select></label>
          <input disabled={progress !== null} type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" aria-label="Spreadsheet file" onChange={(e) => { const f = e.target.files?.[0]; if (f) void load(f); }} />
        </div>
      </section>

      {error && <p role="alert" className="error">{error}</p>}
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
                    <select disabled={progress !== null} aria-label={`Column ${h}`} value={mapping[i] ?? ""} onChange={(e) => { setMapping((m) => m.map((v, j) => (j === i ? e.target.value : v))); }}>
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
            <label className="row"><input type="checkbox" disabled={progress !== null} checked={createCompanies} onChange={(e) => { setCreateCompanies(e.target.checked); }} /> Add companies that do not exist yet</label>
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
