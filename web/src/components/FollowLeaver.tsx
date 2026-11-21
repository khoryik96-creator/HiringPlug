import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { companiesKey, contactsKey, jobsKey } from "../api/keys";
import { linkedinDates } from "../labels";
import { afterContactAdded, type CompanyMatch, companiesNamed } from "./contactLinkedin";
import { ErrorMessage } from "./ErrorMessage";

/** Where they work now: a LinkedIn role (with its company page), or the current job on a profile PDF (no page). */
export type Move = Schemas["LinkedinMove"];

const slugOf = (companyUrl: string) => /\/company\/([A-Za-z0-9_-]+)\//.exec(companyUrl)?.[1] ?? "";

/** The company to add them at: one in HiringPlug, or a new one (made when chosen). */
type Target = { id: string; name: string } | "new";

/**
 * Following a contact who left to where they work now. The company in
 * HiringPlug with the same LinkedIn page is certain, so they are added there
 * at once; otherwise the companies whose names match are offered, and a new
 * company is made only when the person says so. They are added as a contact
 * there (an email guessed from colleagues', then verified), with a note on the
 * company that a known contact joined it: a business lead.
 */
export function FollowLeaver({ contactId, move }: { contactId: string; move: Move }) {
  const queryClient = useQueryClient();
  const [choices, setChoices] = useState<CompanyMatch[] | null>(null);
  const find = useMutation({
    mutationFn: async (): Promise<{ page: { id: string; name: string } | undefined; named: CompanyMatch[] }> => {
      const slug = slugOf(move.companyUrl);
      const page = slug ? unwrap(await api.GET("/v1/companies", { params: { query: { linkedinCompany: slug, limit: 1 } } })).items[0] : undefined;
      return { page, named: page ? [] : await companiesNamed(move.companyName, move.companyUrl) };
    },
    onSuccess: ({ page, named }) => {
      if (page) follow.mutate(page);
      else setChoices(named);
    },
  });
  const follow = useMutation({
    mutationFn: async (target: Target) => {
      const old = unwrap(await api.GET("/v1/contacts/{contactId}", { params: { path: { contactId } } }));
      const isNew = target === "new";
      const company = isNew ? unwrap(await api.POST("/v1/companies", { body: { name: move.companyName, ...(move.companyUrl ? { linkedinUrl: move.companyUrl } : {}) } })) : target;
      if (!isNew) {
        // Followed already (or added by hand): not twice.
        const there = unwrap(await api.GET("/v1/contacts", { params: { query: { companyId: company.id, q: old.fullName, limit: 20 } } })).items
          .find((c) => c.fullName.toLowerCase() === old.fullName.toLowerCase());
        if (there) return { company, added: there, isNew, already: true };
        if (move.companyUrl) await notePage(company.id, move.companyUrl);
      }
      const since = move.since ? ` in ${linkedinDates(move.since)}` : "";
      const added = unwrap(await api.POST("/v1/contacts", {
        body: {
          companyId: company.id, firstName: old.firstName, lastName: old.lastName, title: move.title || old.title,
          linkedinUrl: old.linkedinUrl, role: old.role,
          notes: `Moved from ${old.companyName}${since} (seen on LinkedIn).`,
        },
      }));
      unwrap(await api.POST("/v1/notes", {
        body: {
          subjectType: "company", subjectId: company.id,
          body: `Lead: ${old.fullName}${old.title ? `, ${old.title}` : ""} at ${old.companyName} (our contact), joined ${move.companyName}${since}${move.title ? ` as ${move.title}` : ""}.`,
        },
      }));
      // Their old address is the old company's: one is guessed from their new colleagues' (and verified).
      await afterContactAdded(added, false).catch(() => undefined);
      return { company, added, isNew, already: false };
    },
    onSuccess: async () => {
      setChoices(null);
      await queryClient.invalidateQueries({ queryKey: contactsKey });
      await queryClient.invalidateQueries({ queryKey: companiesKey });
    },
  });
  if (follow.data) {
    const { company, added, isNew, already } = follow.data;
    return (
      <span className="follow-done" role="status">
        {already ? "Already a contact: " : "Added: "}
        <a href={`#/contacts/${added.id}`}>{added.fullName}</a> at <a href={`#/companies/${company.id}`}>{company.name}</a>
        {already ? "" : isNew ? " (new company, with a lead note)" : " (with a lead note)"}
      </span>
    );
  }
  const busy = find.isPending || follow.isPending;
  if (choices) {
    return (
      <span className="follow-choose" role="group" aria-label={`Where ${move.companyName} is in HiringPlug`}>
        {choices.length > 0
          ? <span className="muted small">Is it one of these?</span>
          : <span className="muted small">{move.companyName} is not in HiringPlug yet.</span>}
        {choices.map((c) => (
          <button key={c.id} type="button" className="small-button" disabled={busy} onClick={() => { follow.mutate(c); }}>
            Add at {c.name}{c.detail ? ` (${c.detail})` : ""}
          </button>
        ))}
        <button type="button" className={`small-button${choices.length ? " secondary" : ""}`} disabled={busy} onClick={() => { follow.mutate("new"); }}>
          {choices.length ? `None of these: create ${move.companyName}` : `Create ${move.companyName} and add them`}
        </button>
        <button type="button" className="link small" disabled={busy} onClick={() => { setChoices(null); }}>Cancel</button>
        <ErrorMessage error={follow.error} />
      </span>
    );
  }
  return (
    <span className="follow-leaver">
      <button type="button" className="small-button" disabled={busy} onClick={() => { find.mutate(); }}>
        {busy ? "Adding…" : `Add at ${move.companyName}`}
      </button>
      <ErrorMessage error={find.error ?? follow.error} />
    </span>
  );
}

/** A company chosen by name that has no LinkedIn page yet gets theirs, so the next move is found by it. */
async function notePage(companyId: string, page: string): Promise<void> {
  try {
    const c = unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId } } }));
    if (!c.linkedinUrl) await api.PATCH("/v1/companies/{companyId}", { params: { path: { companyId } }, body: { version: c.version, linkedinUrl: page } });
  } catch { /* a nicety: the contact is added all the same */ }
}

/** Where a contact who left works now, each with "Add at …". */
export function LeaverMoves({ contactId, moves }: { contactId: string; moves: Move[] }) {
  if (moves.length === 0) return null;
  return (
    <ul className="leaver-moves">
      {moves.map((m) => (
        <li key={m.companyUrl}>
          Now {m.title ? `${m.title} at ` : "at "}<a href={m.companyUrl} target="_blank" rel="noreferrer noopener">{m.companyName}</a>
          {m.since && <span className="muted"> since {linkedinDates(m.since)}</span>}{" "}
          <FollowLeaver contactId={contactId} move={m} />
        </li>
      ))}
    </ul>
  );
}

/** The open jobs a contact is the hiring contact for: each needs a new one when they leave. */
export function JobsNeedingContact({ contactId }: { contactId: string }) {
  const jobs = useQuery({
    queryKey: [...jobsKey, "contact", contactId],
    queryFn: async () => {
      const [open, hold] = await Promise.all((["open", "on_hold"] as const).map(async (status) =>
        unwrap(await api.GET("/v1/jobs", { params: { query: { contactId, status, limit: 100 } } })).items));
      return [...(open ?? []), ...(hold ?? [])];
    },
  });
  if (!jobs.data?.length) return null;
  return (
    <p className="warning small jobs-needing-contact" role="status">
      Hiring contact on {jobs.data.length === 1 ? "1 open job, which needs" : `${String(jobs.data.length)} open jobs, which need`} a new contact:{" "}
      {jobs.data.map((j, i) => <span key={j.id}>{i > 0 && ", "}<a href={`#/jobs/${j.id}`}>{j.title}</a></span>)}
    </p>
  );
}
