import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { candidateApplicationsKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { SaveError } from "./SaveError";

export const cvTemplatesKey = ["cv-templates"] as const;

/**
 * Make a formatted CV: the candidate's profile in a Word template, saved with
 * their documents (a PDF follows). Contact details stay out unless asked.
 */
export function FormatCv({ candidateId, permissions, onDone }: { candidateId: string; permissions: Permission[]; onDone: () => void }) {
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
    <form className="panel form-grid" aria-label="Make formatted CV" onSubmit={(e) => { e.preventDefault(); make.mutate(); }}>
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
        Blind CV: a reference instead of the name; no contact details or nationality; names, emails, phone numbers and links taken out of the text (checked before saving)
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
