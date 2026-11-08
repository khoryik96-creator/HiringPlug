// The advanced candidate search: a boolean search of their CVs and profiles,
// and dropdowns to narrow by status, work authorization, where they are
// from and live, source, industry, skills, notice and expected salary.
import { useState } from "react";

import { COUNTRIES } from "../forms";
import { countryName } from "../../labels";
import { AVAILABILITY } from "../candidateLook";
import { useFieldSettings } from "../useFieldSettings";
import { useUsedValues } from "../useUsedValues";
import { booleanTokens, EMPTY, NOTICE_CHOICES, type SearchState, UPDATED_CHOICES, WORK_KINDS } from "./candidateSearch";
import { MultiSelect } from "./MultiSelect";

const COUNTRY_CHOICES = COUNTRIES.map((c) => ({ value: c, label: countryName(c) }));

export function AdvancedSearch({ value, canSeeSalary, onSearch, onClose }: {
  value: SearchState;
  canSeeSalary: boolean;
  onSearch: (s: SearchState) => void;
  onClose: () => void;
}) {
  const [draft, setDraft] = useState(value);
  const used = useUsedValues();
  const fs = useFieldSettings();
  const set = <K extends keyof SearchState>(k: K) => (v: SearchState[K]) => { setDraft((d) => ({ ...d, [k]: v })); };
  const plain = (vs: string[]) => vs.map((v) => ({ value: v, label: v }));
  // Values already in use, plus any chosen before that are no longer used.
  const withChosen = (vs: string[], chosen: string[]) => plain([...new Set([...vs, ...chosen])]);
  return (
    <form className="panel adv-search" aria-label="Advanced search" onSubmit={(e) => { e.preventDefault(); onSearch(draft); }}>
      <div className="adv-head">
        <h3>Advanced search</h3>
        <button type="button" className="link" onClick={onClose}>Close</button>
      </div>

      <label className="adv-boolean">
        <span className="adv-label">Boolean search <span className="muted small">in their CVs and profiles</span></span>
        <input value={draft.boolean} placeholder='SAP AND (BW OR BPC) NOT internship' spellCheck={false}
          onChange={(e) => { set("boolean")(e.target.value); }} />
      </label>
      {draft.boolean.trim() ? (
        <p className="bool-preview" aria-label="How it is read">
          {booleanTokens(draft.boolean).map((t, i) => <span key={i} className={`bool-${t.kind}`}>{t.text}</span>)}
        </p>
      ) : (
        <p className="muted small adv-help">
          <b>AND</b> both · <b>OR</b> either · <b>NOT</b> or <b>-word</b> leaves out · <b>( )</b> groups · <b>&quot;exact phrase&quot;</b> · <b>manag*</b> any ending.
          Words side by side must all appear.
        </p>
      )}

      <div className="adv-grid">
        <MultiSelect label="Status" choices={Object.entries(AVAILABILITY).map(([v, a]) => ({ value: v, label: a.label }))}
          value={draft.availability} onChange={(v) => { set("availability")(v as SearchState["availability"]); }} />
        <MultiSelect label="Work authorization" choices={WORK_KINDS} value={draft.workAuthorization}
          onChange={(v) => { set("workAuthorization")(v as SearchState["workAuthorization"]); }} />
        <MultiSelect label="Nationality" choices={COUNTRY_CHOICES} value={draft.nationality} onChange={set("nationality")} />
        <MultiSelect label="Lives in" choices={COUNTRY_CHOICES} value={draft.residence} onChange={set("residence")} />
        <MultiSelect label="Source" choices={fs.options("candidate_source").map((o) => ({ value: o.key, label: o.label }))}
          value={draft.source} onChange={set("source")} />
        <MultiSelect label="Industry" choices={withChosen(used.industries, draft.industry)} value={draft.industry} onChange={set("industry")} />
        <div className="adv-skills">
          <MultiSelect label="Skills" choices={withChosen(used.skills, draft.skills)} value={draft.skills} onChange={set("skills")} />
          {draft.skills.length > 1 && (
            <div className="seg" role="group" aria-label="Skills needed">
              <button type="button" aria-pressed={draft.skillsMatch === "all"} onClick={() => { set("skillsMatch")("all"); }}>All</button>
              <button type="button" aria-pressed={draft.skillsMatch === "any"} onClick={() => { set("skillsMatch")("any"); }}>Any</button>
            </div>
          )}
        </div>
        <MultiSelect label="Tags" choices={withChosen(used.tags, draft.tags)} value={draft.tags} onChange={set("tags")} />
        <label className="adv-field"><span className="multi-label">Location</span>
          <input value={draft.location} placeholder="e.g. Petaling Jaya" onChange={(e) => { set("location")(e.target.value); }} />
        </label>
        <label className="adv-field"><span className="multi-label">Notice period</span>
          <select value={draft.noticeMaxDays ?? ""} onChange={(e) => { set("noticeMaxDays")(e.target.value === "" ? null : Number(e.target.value)); }}>
            <option value="">Any</option>
            {NOTICE_CHOICES.map((n) => <option key={n.days} value={n.days}>{n.label}</option>)}
          </select>
        </label>
        {canSeeSalary && (
          <label className="adv-field"><span className="multi-label">Expects at most (RM / month)</span>
            <input inputMode="numeric" value={draft.salaryMax} placeholder="e.g. 12000" onChange={(e) => { set("salaryMax")(e.target.value); }} />
          </label>
        )}
        <label className="adv-field"><span className="multi-label">Profile updated</span>
          <select value={draft.updatedWithinDays ?? ""} onChange={(e) => { set("updatedWithinDays")(e.target.value === "" ? null : Number(e.target.value)); }}>
            <option value="">Any time</option>
            {UPDATED_CHOICES.map((u) => <option key={u.days} value={u.days}>{u.label}</option>)}
          </select>
        </label>
      </div>

      <div className="adv-actions">
        <button type="submit">Search</button>
        <button type="button" className="link" onClick={() => { setDraft(EMPTY); onSearch(EMPTY); }}>Clear all</button>
      </div>
    </form>
  );
}
