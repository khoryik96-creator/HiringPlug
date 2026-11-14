import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { companiesKey, contactsKey, jobsKey } from "../api/keys";
import { linkedinDates } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Move = Schemas["LinkedinMove"];

const slugOf = (companyUrl: string) => /\/company\/([A-Za-z0-9_-]+)\//.exec(companyUrl)?.[1] ?? "";

/**
 * Following a contact who left to where they work now: adds them as a contact
 * there (the company too, if it is new), with a note on the company that a
 * known contact joined it: a business lead.
 */
export function FollowLeaver({ contactId, move }: { contactId: string; move: Move }) {
  const queryClient = useQueryClient();
  const follow = useMutation({
    mutationFn: async () => {
      const old = unwrap(await api.GET("/v1/contacts/{contactId}", { params: { path: { contactId } } }));
      const found = unwrap(await api.GET("/v1/companies", { params: { query: { linkedinCompany: slugOf(move.companyUrl), limit: 1 } } })).items[0];
      const company = found ?? unwrap(await api.POST("/v1/companies", { body: { name: move.companyName, linkedinUrl: move.companyUrl } }));
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
      return { company, added, isNew: !found };
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: contactsKey });
      await queryClient.invalidateQueries({ queryKey: companiesKey });
    },
  });
  if (follow.data) {
    const { company, added, isNew } = follow.data;
    return (
      <span className="follow-done" role="status">
        Added: <a href={`#/contacts/${added.id}`}>{added.fullName}</a> at <a href={`#/companies/${company.id}`}>{company.name}</a>
        {isNew ? " (new company, with a lead note)" : " (with a lead note)"}
      </span>
    );
  }
  return (
    <span className="follow-leaver">
      <button type="button" className="small-button" disabled={follow.isPending} onClick={() => { follow.mutate(); }}>
        {follow.isPending ? "Adding…" : `Add at ${move.companyName}`}
      </button>
      <ErrorMessage error={follow.error} />
    </span>
  );
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
      Hiring contact on {jobs.data.length === 1 ? "1 open job" : `${String(jobs.data.length)} open jobs`}, which need a new contact:{" "}
      {jobs.data.map((j, i) => <span key={j.id}>{i > 0 && ", "}<a href={`#/jobs/${j.id}`}>{j.title}</a></span>)}
    </p>
  );
}
