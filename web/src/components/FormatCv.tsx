import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Document, type Permission, type Schemas, unwrap } from "../api/client";
import { candidateApplicationsKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { fieldErrors } from "./forms";
import { SaveError } from "./SaveError";

export const cvTemplatesKey = ["cv-templates"] as const;

export const studioCvsKey = (candidateId: string) => ["studio-cvs", candidateId] as const;

const READABLE = new Set(["application/pdf", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/msword"]);

/**
 * Make a formatted CV, saved with the candidate's documents (a PDF follows):
 * from one of their own CVs, CV Studio's way (the AI reads it; done in the
 * background), or from their profile in a Word template. Contact details stay
 * out unless asked.
 */
export function FormatCv({ candidateId, permissions, onDone }: { candidateId: string; permissions: Permission[]; onDone: () => void }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const cvs = (docs.data ?? []).filter((d) => d.source !== "formatted" && READABLE.has(d.contentType));
  const [picked, setPicked] = useState<"studio" | "template" | null>(null);
  const mode = picked ?? (cvs.length > 0 ? "studio" : "template");
  return (
    <div className="panel format-cv">
      <div className="choice" role="radiogroup" aria-label="Make it from">
        <label className={mode === "studio" ? "on" : undefined}>
          <input type="radio" name="format-mode" checked={mode === "studio"} disabled={cvs.length === 0} onChange={() => { setPicked("studio"); }} />
          <span><strong>From their CV</strong><small>CV Studio&apos;s layout and rules; the AI reads the CV</small></span>
        </label>
        <label className={mode === "template" ? "on" : undefined}>
          <input type="radio" name="format-mode" checked={mode === "template"} onChange={() => { setPicked("template"); }} />
          <span><strong>From their profile</strong><small>The profile in a Word template</small></span>
        </label>
      </div>
      {mode === "studio"
        ? <StudioForm candidateId={candidateId} cvs={cvs} onDone={onDone} />
        : <TemplateForm candidateId={candidateId} permissions={permissions} onDone={onDone} />}
    </div>
  );
}

type SummaryLength = Exclude<Schemas["StudioCvSummary"], "none">;
type SummaryFocus = Schemas["StudioCvSummaryFocus"];
/** How the About Him / Her summary is written: remembered in this browser for the next CV. */
export type SummaryChoice = { write: boolean; length: SummaryLength; focus: SummaryFocus; direction: string };

const SUMMARY_KEY = "hiringplug.studio.summary";
const SUMMARY_DEFAULT: SummaryChoice = { write: true, length: "normal", focus: "general", direction: "" };
const LENGTHS: [SummaryLength, string][] = [["short", "Short (4-5)"], ["normal", "Normal (6-7)"], ["detailed", "Detailed (8-10)"]];
const FOCUSES: [SummaryFocus, string][] = [["general", "General focus"], ["technology", "Technology"], ["leadership", "Leadership"], ["commercial", "Commercial"]];

export function loadSummaryChoice(): SummaryChoice {
  try {
    const saved = JSON.parse(localStorage.getItem(SUMMARY_KEY) ?? "{}") as Partial<SummaryChoice>;
    return {
      write: typeof saved.write === "boolean" ? saved.write : SUMMARY_DEFAULT.write,
      length: LENGTHS.some(([k]) => k === saved.length) ? saved.length as SummaryLength : SUMMARY_DEFAULT.length,
      focus: FOCUSES.some(([k]) => k === saved.focus) ? saved.focus as SummaryFocus : SUMMARY_DEFAULT.focus,
      direction: typeof saved.direction === "string" ? saved.direction.slice(0, 500) : "",
    };
  } catch { return SUMMARY_DEFAULT; }
}

function saveSummaryChoice(choice: SummaryChoice) {
  try { localStorage.setItem(SUMMARY_KEY, JSON.stringify(choice)); } catch { /* private window: not remembered */ }
}

function StudioForm({ candidateId, cvs, onDone }: { candidateId: string; cvs: Document[]; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({
    documentId: "", blind: false, neutralGender: false, correctLanguage: true, alignment: "left" as "left" | "justify",
  });
  const [summary, setSummary] = useState(loadSummaryChoice);
  const documentId = form.documentId || cvs[0]?.id || "";
  const writes = summary.write && !form.blind;
  const start = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/candidates/{candidateId}/studio-cvs", {
      params: { path: { candidateId } },
      body: {
        documentId, blind: form.blind, neutralGender: form.blind && form.neutralGender, correctLanguage: form.correctLanguage,
        alignment: form.alignment, summary: writes ? summary.length : "none",
        ...(writes ? { summaryFocus: summary.focus, ...(summary.direction.trim() ? { summaryDirection: summary.direction.trim() } : {}) } : {}),
      },
    })),
    onSuccess: async () => {
      if (!form.blind) saveSummaryChoice(summary);
      await queryClient.invalidateQueries({ queryKey: studioCvsKey(candidateId) });
      onDone();
    },
  });
  return (
    <form className="form-grid" aria-label="Format their CV" onSubmit={(e) => { e.preventDefault(); start.mutate(); }}>
      <label className="field full">CV
        <select value={documentId} onChange={(e) => { setForm({ ...form, documentId: e.target.value }); }}>
          {cvs.map((d) => <option key={d.id} value={d.id}>{d.fileName} ({d.source}, {new Date(d.createdAt).toLocaleDateString("en-GB")})</option>)}
        </select>
      </label>
      <label className="row full">
        <input type="checkbox" checked={form.correctLanguage} onChange={(e) => { setForm({ ...form, correctLanguage: e.target.checked }); }} />{" "}
        Correct spelling, punctuation and capitals (never the meaning, numbers or dates)
      </label>
      <label className="row full">
        <input type="checkbox" checked={form.blind} onChange={(e) => { setForm({ ...form, blind: e.target.checked }); }} />{" "}
        Blind CV (no name or contact details; employers and schools described, not named)
      </label>
      {form.blind && (
        <label className="row full">
          <input type="checkbox" checked={form.neutralGender} onChange={(e) => { setForm({ ...form, neutralGender: e.target.checked }); }} />{" "}
          &ldquo;The candidate&rdquo; instead of he or she
        </label>
      )}
      {!form.blind && <SummaryOptions choice={summary} onChange={setSummary} error={fieldErrors(start.error).summaryDirection} />}
      <label className="field">Paragraphs
        <select value={form.alignment} onChange={(e) => { setForm({ ...form, alignment: e.target.value === "justify" ? "justify" : "left" }); }}>
          <option value="left">Aligned left</option>
          <option value="justify">Justified</option>
        </select>
      </label>
      <ErrorMessage error={fieldErrors(start.error).summaryDirection ? null : start.error} />
      <div className="row full">
        <button type="submit" disabled={start.isPending || !documentId}>Format CV</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}

/** The About Him / Her summary: whether the AI writes one, and how (CV Studio's lengths and focus, and your own direction and tone). */
function SummaryOptions({ choice, onChange, error }: { choice: SummaryChoice; onChange: (c: SummaryChoice) => void; error?: string }) {
  return (
    <div className="full summary-options">
      <label className="row">
        <input type="checkbox" checked={choice.write} onChange={(e) => { onChange({ ...choice, write: e.target.checked }); }} /> AI summary
      </label>
      {choice.write && (
        <>
          <select aria-label="Summary length" value={choice.length} onChange={(e) => { onChange({ ...choice, length: e.target.value as SummaryLength }); }}>
            {LENGTHS.map(([key, label]) => <option key={key} value={key}>{label}</option>)}
          </select>
          <select aria-label="Summary focus" value={choice.focus} onChange={(e) => { onChange({ ...choice, focus: e.target.value as SummaryFocus }); }}>
            {FOCUSES.map(([key, label]) => <option key={key} value={key}>{label}</option>)}
          </select>
          <input className="summary-direction" aria-label="Direction and tone" maxLength={500} value={choice.direction}
            placeholder="Direction & tone (optional)" title="e.g. Confident, concise; lead with team leadership"
            onChange={(e) => { onChange({ ...choice, direction: e.target.value }); }} />
        </>
      )}
      {error && <small className="error">{error}</small>}
    </div>
  );
}

function TemplateForm({ candidateId, permissions, onDone }: { candidateId: string; permissions: Permission[]; onDone: () => void }) {
  const queryClient = useQueryClient();
  const templates = useQuery({ queryKey: cvTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/cv-templates")).items });
  const apps = useQuery({
    queryKey: candidateApplicationsKey(candidateId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/applications", { params: { path: { candidateId } } })),
    enabled: permissions.includes("pipeline:read"),
  });
  const [form, setForm] = useState({ templateId: "standard", applicationId: "", blind: false, showContact: false, includeSalary: false, summary: "" });
  const make = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/candidates/{candidateId}/formatted-cvs", {
      params: { path: { candidateId } },
      body: {
        templateId: form.templateId, blind: form.blind, showContact: form.showContact && !form.blind, includeSalary: form.includeSalary,
        ...(form.applicationId ? { applicationId: form.applicationId } : {}), ...(form.summary.trim() ? { summary: form.summary.trim() } : {}),
      },
    })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["documents", candidateId] });
      onDone();
    },
  });
  return (
    <form className="form-grid" aria-label="Make formatted CV" onSubmit={(e) => { e.preventDefault(); make.mutate(); }}>
      <ErrorMessage error={templates.error} />
      <label className="field">Template
        <select value={form.templateId} onChange={(e) => { setForm({ ...form, templateId: e.target.value }); }}>
          {templates.data?.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
      </label>
      {(apps.data?.items.length ?? 0) > 0 && (
        <label className="field">For
          <select value={form.applicationId} onChange={(e) => { setForm({ ...form, applicationId: e.target.value }); }}>
            <option value="">No particular job</option>
            {apps.data?.items.map((a) => <option key={a.id} value={a.id}>{a.jobTitle} at {a.companyName}</option>)}
          </select>
        </label>
      )}
      <label className="row full">
        <input type="checkbox" checked={form.blind} onChange={(e) => { setForm({ ...form, blind: e.target.checked }); }} />{" "}
        Blind CV (no name, contact details or nationality)
      </label>
      {!form.blind && (
        <label className="row full"><input type="checkbox" checked={form.showContact} onChange={(e) => { setForm({ ...form, showContact: e.target.checked }); }} /> Show email and phone</label>
      )}
      {permissions.includes("candidates.compensation:read") && (
        <label className="row full"><input type="checkbox" checked={form.includeSalary} onChange={(e) => { setForm({ ...form, includeSalary: e.target.checked }); }} /> Include current and expected salary</label>
      )}
      <label className="field full">Your summary for the client (optional)
        <textarea rows={4} value={form.summary} onChange={(e) => { setForm({ ...form, summary: e.target.value }); }} />
      </label>
      <SaveError error={make.error} shown={[]} onReload={onDone} />
      <div className="row full">
        <button type="submit" disabled={make.isPending}>Make formatted CV</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
        {permissions.includes("cvformats.templates:manage") && <a href="#/cv-templates" className="muted">CV templates</a>}
      </div>
    </form>
  );
}
