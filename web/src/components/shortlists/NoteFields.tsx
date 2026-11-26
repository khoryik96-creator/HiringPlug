// "Add a detail" beside each note on a client link: the candidate's expected
// salary, notice period, current title and the like, put in as "Notice
// period: 1 month", as the client email's fields are. Names are not offered
// (a blind CV stays blind), nor email or phone (never on a link). A salary is
// asked for only when chosen, so it is counted as shown, as in an email.
import { useMutation, useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { emailTemplatesKey } from "../../api/keys";

type Field = Schemas["EmailField"];

const NAMES = new Set(["candidate.firstName", "candidate.lastName", "candidate.fullName"]);

/** The fields a note to the client may hold: the candidate's, without their name. */
export function noteFields(fields: Field[]): Field[] {
  return fields.filter((f) => f.key.startsWith("candidate.") && !NAMES.has(f.key));
}

/** The note with "Label: value" added on its own line (or alone when the note is empty). */
export function withDetail(note: string, label: string, value: string): string {
  const line = `${label}: ${value.replace(/\s+/g, " ").trim()}`;
  return note.trim() ? `${note.trimEnd()}\n${line}` : line;
}

export function NoteFields({ candidateId, applicationId, name, note, onChange }: {
  candidateId: string; applicationId: string; name: string; note: string; onChange: (note: string) => void;
}) {
  const templates = useQuery({ queryKey: emailTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/email-templates")) });
  const fields = noteFields(templates.data?.fields ?? []);
  const fill = useMutation({
    mutationFn: async (f: Field) => {
      const out = unwrap(await api.POST("/v1/email-previews", { body: {
        subject: "", bodyHtml: "", candidateId, applicationId, timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Kuala_Lumpur",
        ...(f.salary && { includeSalary: true }),
      } }));
      return { f, value: out.fieldValues[f.token] ?? "" };
    },
    onSuccess: ({ f, value }) => { if (value.trim()) onChange(withDetail(note, f.label, value)); },
  });
  if (fields.length === 0) return null;
  const empty = fill.data && !fill.data.value.trim() ? fill.data.f.label : "";
  return (
    <span className="note-fields">
      <select aria-label={`Add a detail of ${name}`} value="" disabled={fill.isPending}
        onChange={(e) => { const f = fields.find((x) => x.token === e.target.value); if (f) fill.mutate(f); }}>
        <option value="">{fill.isPending ? "Adding…" : "+ Add a detail…"}</option>
        {fields.map((f) => <option key={f.token} value={f.token}>{f.label}</option>)}
      </select>
      {empty && <span className="muted small" role="status">{empty}: not filled in on their page.</span>}
      {fill.error && <span className="error small" role="alert">Could not add it. Try again.</span>}
    </span>
  );
}
