import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Schemas, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { applyBody, historyFromProfile, newSkills, suggestions } from "./cvSuggestions";
import { ErrorMessage } from "./ErrorMessage";
import { DuplicateEmail } from "./DuplicateEmail";
import { SaveError } from "./SaveError";

/**
 * Details read from a CV, next to what the profile has now. The recruiter
 * ticks what to use; empty profile fields are ticked by default, filled ones
 * never are. Each value shows the CV text it came from.
 */
export function CvReview({ documentId, candidate, onDone }: { documentId: string; candidate: Candidate; onDone: () => void }) {
  const doc = useQuery({
    queryKey: ["document", documentId],
    queryFn: async () => unwrap(await api.GET("/v1/documents/{documentId}", { params: { path: { documentId } } })),
  });
  const profile = doc.data?.parsedProfile;
  if (doc.error) return <ErrorMessage error={doc.error} />;
  if (!doc.data) return <p className="muted">Loading…</p>;
  if (!profile) return <p className="muted">No details were read from this CV.</p>;
  return <Review profile={profile} candidate={candidate} onDone={onDone} />;
}

function Review({ profile, candidate, onDone }: { profile: Schemas["ParsedProfile"]; candidate: Candidate; onDone: () => void }) {
  const queryClient = useQueryClient();
  // Captured when the review opens: a background refresh must not change what is offered.
  const [opened] = useState(() => ({ candidate, rows: suggestions(profile, candidate), skills: newSkills(profile, candidate) }));
  const [ticked, setTicked] = useState(() => new Set(opened.rows.filter((r) => r.checked).map((r) => r.key)));
  const [skills, setSkills] = useState(() => new Set(opened.skills.map((s) => s.value)));
  const toggle = <T,>(set: Set<T>, value: T) => {
    const next = new Set(set);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    return next;
  };

  const cvHistory = historyFromProfile(profile);
  const hasHistory = cvHistory.experience.length + cvHistory.education.length > 0;
  const profileHasHistory = opened.candidate.experience.length + opened.candidate.education.length > 0;
  // Ticked by default only when the profile has no history: never replaced unasked.
  const [useHistory, setUseHistory] = useState(() => hasHistory && !profileHasHistory);

  const apply = useMutation({
    mutationFn: async () => {
      let saved = opened.candidate;
      if (ticked.size > 0 || skills.size > 0) {
        saved = unwrap(
          await api.PATCH("/v1/candidates/{candidateId}", {
            params: { path: { candidateId: candidate.id } },
            body: applyBody(profile, opened.candidate, ticked, [...skills]),
          }),
        );
      }
      if (useHistory) {
        saved = unwrap(
          await api.PUT("/v1/candidates/{candidateId}/history", {
            params: { path: { candidateId: candidate.id } },
            body: { version: saved.version, ...cvHistory },
          }),
        );
      }
      return saved;
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(candidateKey(saved.id), saved);
      onDone();
    },
    // A partial save (details saved, history refused) still changed the profile.
    onError: () => queryClient.invalidateQueries({ queryKey: candidateKey(candidate.id) }),
  });
  const nothing = ticked.size === 0 && skills.size === 0 && !useHistory;

  return (
    <section className="panel" aria-label="Details from this CV">
      <h4>Details from this CV</h4>
      {opened.rows.length === 0 && opened.skills.length === 0 && <p className="muted">The profile already has everything this CV says.</p>}
      {opened.rows.length > 0 && (
        <table>
          <thead>
            <tr>
              <th>Use</th>
              <th>Field</th>
              <th>From the CV</th>
              <th>Profile now</th>
            </tr>
          </thead>
          <tbody>
            {opened.rows.map((r) => (
              <tr key={r.key}>
                <td>
                  <input type="checkbox" aria-label={`Use ${r.label}`} checked={ticked.has(r.key)} onChange={() => { setTicked(toggle(ticked, r.key)); }} />
                </td>
                <td>{r.label}</td>
                <td>
                  {r.value}
                  {r.quote !== r.value && <div className="muted">CV says: “{r.quote}”</div>}
                </td>
                <td className="muted">{r.current ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {opened.skills.length > 0 && (
        <fieldset>
          <legend>New skills</legend>
          <div className="chips">
            {opened.skills.map((s) => (
              <label key={s.value} className="chip" title={`CV says: “${s.quote}”`}>
                <input type="checkbox" checked={skills.has(s.value)} onChange={() => { setSkills(toggle(skills, s.value)); }} /> {s.value}
              </label>
            ))}
          </div>
        </fieldset>
      )}
      <History profile={profile} />
      {hasHistory && (
        <label className="row">
          <input type="checkbox" checked={useHistory} onChange={(e) => { setUseHistory(e.target.checked); }} />{" "}
          {profileHasHistory
            ? "Replace the profile's work history and education with these"
            : "Save this work history and education on the profile"}
        </label>
      )}
      <div className="row">
        <button type="button" disabled={nothing || apply.isPending} onClick={() => { apply.mutate(); }}>
          Use ticked details
        </button>
        <button type="button" className="link" onClick={onDone}>Close</button>
      </div>
      <SaveError error={apply.error} shown={[]} onReload={() => { void queryClient.invalidateQueries({ queryKey: candidateKey(candidate.id) }); onDone(); }} />
      <DuplicateEmail error={apply.error} />
      {profile.dropped > 0 && (
        <p className="muted">{profile.dropped} value(s) were left out because the CV text did not support them.</p>
      )}
    </section>
  );
}

function History({ profile }: { profile: Schemas["ParsedProfile"] }) {
  if (profile.experience.length === 0 && profile.education.length === 0) return null;
  return (
    <div>
      {profile.experience.length > 0 && (
        <>
          <h5>Work history in this CV</h5>
          <ul>
            {profile.experience.map((j, i) => (
              <li key={i}>
                {[j.title?.value, j.company?.value].filter(Boolean).join(" — ")}{" "}
                <span className="muted">{j.dates?.value}</span>
              </li>
            ))}
          </ul>
        </>
      )}
      {profile.education.length > 0 && (
        <>
          <h5>Education in this CV</h5>
          <ul>
            {profile.education.map((e, i) => (
              <li key={i}>
                {[e.qualification?.value, e.institution?.value].filter(Boolean).join(" — ")}{" "}
                <span className="muted">{e.dates?.value}</span>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
