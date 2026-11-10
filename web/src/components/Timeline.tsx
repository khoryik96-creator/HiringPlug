import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { activityKey, notesKey, screeningCallsKey } from "../api/keys";
import { formatDay, keysOf, NOTE_KIND } from "../labels";
import { describe } from "./activityText";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { fieldErrors } from "./forms";

type Subject = Schemas["NoteSubjectType"];

/** Questions to start a screening note with; edit them freely. */
export const SCREENING_TEMPLATE = [
  "Current role and reason for leaving:",
  "Notice period and availability:",
  "Current and expected salary:",
  "Work authorization:",
  "Key skills for this role:",
  "Concerns:",
].join("\n");

const when = (iso: string) => new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });

type Note = Schemas["Note"];
type Filter = "all" | "calls" | "references" | "notes" | "changes";
const FILTERS: { key: Filter; label: string }[] = [
  { key: "all", label: "All" }, { key: "calls", label: "Calls" }, { key: "references", label: "Reference checks" },
  { key: "notes", label: "Notes" }, { key: "changes", label: "Changes" },
];
const isCall = (n: Note) => n.kind === "screening" || n.kind === "call";
const filterOf = (n: Note): Filter => (isCall(n) ? "calls" : n.kind === "reference" ? "references" : "notes");
/** A note longer than this is folded to its first lines. */
const LONG = 220;

/** A call's lines "RFL: wants regional scope" as label and answer; its "Call on … by …" line is in the header. */
export function callLines(body: string): { label: string | null; text: string }[] {
  return body.split("\n").filter((l) => l.trim() && !/^Call on .+ by .+$/.test(l.trim())).map((l) => {
    const m = /^([^:\n]{1,40}):\s+(.+)$/.exec(l.trim());
    return m ? { label: m[1] ?? null, text: m[2] ?? "" } : { label: null, text: l.trim() };
  });
}

/**
 * Notes on a record and what changed on it: the latest call pinned on top,
 * then everything newest first, filtered by kind and searchable, long notes
 * folded to their first lines.
 */
export function Timeline({ subjectType, subjectId, permissions }: { subjectType: Subject; subjectId: string; permissions: Permission[] }) {
  const canWrite = permissions.includes("notes:create");
  const canSensitive = permissions.includes("notes.sensitive:read");
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState<Set<string>>(new Set());
  const notes = useQuery({
    queryKey: notesKey(subjectType, subjectId),
    queryFn: async () => unwrap(await api.GET("/v1/notes", { params: { query: { subjectType, subjectId } } })),
    enabled: permissions.includes("notes:read"),
  });
  const activity = useQuery({
    queryKey: activityKey(subjectType, subjectId),
    queryFn: async () => unwrap(await api.GET("/v1/activity", { params: { query: { entityType: subjectType, entityId: subjectId } } })),
    enabled: permissions.includes("notes.activity:read"),
  });
  // A candidate's calls say when each was and who made it (logged later, or by someone else).
  const calls = useQuery({
    queryKey: screeningCallsKey(subjectId),
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/screening-calls", { params: { path: { candidateId: subjectId } } })).items,
    enabled: subjectType === "candidate",
  });

  const allNotes = notes.data?.items ?? [];
  const latestCall = calls.data?.find((c) => c.noteId && allNotes.some((n) => n.id === c.noteId));
  const pinned: { note: Note; on: string; by: string } | null = latestCall
    ? { note: allNotes.find((n) => n.id === latestCall.noteId) as Note, on: formatDay(latestCall.calledOn), by: latestCall.calledByName ?? "someone" }
    : (() => {
      const n = allNotes.find(isCall); // newest first
      return n ? { note: n, on: when(n.createdAt), by: n.authorName } : null;
    })();
  const callCount = allNotes.filter(isCall).length;

  const items = [
    ...allNotes.map((n) => ({ key: n.id, at: n.createdAt, note: n, text: null as string | null, who: n.authorName, kind: filterOf(n) })),
    ...(activity.data?.items ?? []).flatMap((a) => {
      const text = describe(a);
      return text ? [{ key: a.id, at: a.at, note: null, text, who: a.actorName, kind: "changes" }] : [];
    }),
  ].sort((x, y) => y.at.localeCompare(x.at));
  const counts = Object.fromEntries(FILTERS.map((f) => [f.key, f.key === "all" ? items.length : items.filter((i) => i.kind === f.key).length])) as Record<Filter, number>;
  const q = search.trim().toLowerCase();
  const shown = items.filter((i) => {
    if (filter !== "all" && i.kind !== filter) return false;
    if (filter === "all" && !q && pinned && i.key === pinned.note.id) return false; // already on top
    if (!q) return true;
    return `${i.note?.body ?? i.text ?? ""} ${i.who}`.toLowerCase().includes(q);
  });
  const toggle = (key: string) => {
    setOpen((o) => { const next = new Set(o); if (next.has(key)) next.delete(key); else next.add(key); return next; });
  };

  return (
    <section className="panel" aria-label="Notes and activity">
      <h3>Notes and activity</h3>
      {canWrite && <NoteForm subjectType={subjectType} subjectId={subjectId} canSensitive={canSensitive} />}
      <ErrorMessage error={notes.error ?? activity.error} />
      {notes.data && notes.data.hiddenSensitive > 0 && (
        <p className="muted">🔒 {String(notes.data.hiddenSensitive)} sensitive note{notes.data.hiddenSensitive === 1 ? " is" : "s are"} hidden for your role.</p>
      )}
      {pinned && (
        <div className="pinned-call" aria-label="Latest call">
          <span className="pinned-label">
            Latest {pinned.note.kind === "screening" ? "screening call" : "call"} · {pinned.on} · by {pinned.by}
            {pinned.note.sensitive && " · 🔒 sensitive"}
          </span>
          <dl className="call-qa">
            {callLines(pinned.note.body).map((l, i) => (
              l.label ? <div key={i}><dt>{l.label}</dt><dd>{l.text}</dd></div> : <div key={i} className="plain"><dd>{l.text}</dd></div>
            ))}
          </dl>
          {callCount > 1 && (
            <button type="button" className="link small" onClick={() => { setFilter("calls"); setSearch(""); }}>
              {callCount - 1} earlier call{callCount === 2 ? "" : "s"}
            </button>
          )}
        </div>
      )}
      {items.length > 0 && (
        <div className="timeline-tools">
          <div className="chips" role="group" aria-label="Show">
            {FILTERS.filter((f) => f.key === "all" || counts[f.key] > 0).map((f) => (
              <button key={f.key} type="button" className="chip" aria-pressed={filter === f.key} onClick={() => { setFilter(f.key); }}>
                {f.label}<span className="n">{counts[f.key]}</span>
              </button>
            ))}
          </div>
          <input type="search" className="timeline-search" aria-label="Search notes" placeholder="Search notes, e.g. SAP or relocate"
            value={search} onChange={(e) => { setSearch(e.target.value); }} />
        </div>
      )}
      <ul className="timeline">
        {shown.map((i) => {
          const long = (i.note?.body.length ?? 0) > LONG || (i.note?.body.split("\n").length ?? 0) > 4;
          const isOpen = open.has(i.key);
          return (
            <li key={i.key} className={i.note ? "note" : "change"}>
              <div className="muted">
                {i.note && <span className={`kind-tag k-${i.kind}`}>{NOTE_KIND[i.note.kind]}</span>}
                {i.who} · {when(i.at)}
                {i.note?.sensitive && " · 🔒 sensitive"}
              </div>
              {i.note ? (
                <>
                  <p className={`pre${long && !isOpen ? " folded" : ""}`}>{i.note.body}</p>
                  {long && <button type="button" className="link small" onClick={() => { toggle(i.key); }}>{isOpen ? "Show less" : "Show more"}</button>}
                </>
              ) : <p>{i.who} {i.text}</p>}
            </li>
          );
        })}
        {items.length === 0 && <li className="muted">Nothing yet.</li>}
        {items.length > 0 && shown.length === 0 && <li className="muted">Nothing matches{q ? ` “${search.trim()}”` : ""}.</li>}
      </ul>
    </section>
  );
}

function NoteForm({ subjectType, subjectId, canSensitive }: { subjectType: Subject; subjectId: string; canSensitive: boolean }) {
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<Schemas["NoteKind"]>("general");
  const [body, setBody] = useState("");
  const [sensitive, setSensitive] = useState(false);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/notes", { body: { subjectType, subjectId, kind, body, sensitive } })),
    onSuccess: () => {
      setBody("");
      setSensitive(false);
      void queryClient.invalidateQueries({ queryKey: notesKey(subjectType, subjectId) });
    },
  });
  const errors = fieldErrors(save.error);
  return (
    <form className="form-grid" aria-label="Add a note" onSubmit={(e) => { e.preventDefault(); if (body.trim()) save.mutate(); }}>
      <Field label="Kind">
        <select value={kind} onChange={(e) => {
          const k = e.target.value as Schemas["NoteKind"];
          setKind(k);
          if (k === "screening" && body.trim() === "") setBody(SCREENING_TEMPLATE);
        }}>
          {keysOf(NOTE_KIND).map((k) => <option key={k} value={k}>{NOTE_KIND[k]}</option>)}
        </select>
      </Field>
      <div className="full">
        <Field label="Note" error={errors.body}>
          <textarea rows={kind === "screening" ? 7 : 3} value={body} onChange={(e) => { setBody(e.target.value); }} />
        </Field>
      </div>
      <div className="row full">
        <button type="submit" disabled={save.isPending || body.trim() === ""}>Add note</button>
        {canSensitive && (
          <label className="row">
            <input type="checkbox" checked={sensitive} onChange={(e) => { setSensitive(e.target.checked); }} /> Sensitive (only roles that see salaries)
          </label>
        )}
      </div>
      {Object.keys(errors).length === 0 && <div className="full"><ErrorMessage error={save.error} /></div>}
    </form>
  );
}
