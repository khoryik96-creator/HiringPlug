// A candidate's specialization, industries, IT skills and certifications,
// changed right in their Details (no Edit profile): a dropdown beside each,
// saved at once. The person's favourites sit on top of each dropdown.
import { useEffect, useState } from "react";

import type { Candidate } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { FavDropdown } from "./FavDropdown";
import { useFavourites } from "./favourites";
import { type Category, categoryLabel, IndustrySummary, useIndustries } from "./IndustryPicker";
import { Chips } from "./LabelInput";
import { useIndustryManager, useWordListManager } from "./listManagers";
import { useCandidateUpdate } from "./useCandidateUpdate";
import { useFieldSettings } from "./useFieldSettings";

/** Saving, and a short "Saved" once done. */
function useQuickSave(candidate: Candidate) {
  const save = useCandidateUpdate(candidate);
  const [saved, setSaved] = useState(0);
  useEffect(() => {
    if (!saved) return;
    const t = window.setTimeout(() => { setSaved(0); }, 2400);
    return () => { window.clearTimeout(t); };
  }, [saved]);
  const mutate = (change: Parameters<typeof save.mutate>[0]) => { save.mutate(change, { onSuccess: () => { setSaved(Date.now()); } }); };
  const status = <>
    {saved > 0 && <span key={saved} className="quick-saved" role="status">✓ Saved</span>}
    <ErrorMessage error={save.error} />
  </>;
  return { mutate, busy: save.isPending, status };
}

export function QuickSpecialization({ candidate, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const fs = useFieldSettings();
  const favs = useFavourites();
  const save = useQuickSave(candidate);
  const manage = useWordListManager("specialization", favs.of("specialization"));
  if (!canEdit) return <>{candidate.specialization ?? "—"}</>;
  const current = candidate.specialization ?? "";
  return (
    <div className={`quick-edit${current ? "" : " fav-dropdown-add"}`}>
      <FavDropdown inline aria-label={`Specialization: ${current || "not set"}. Change`} text={current || "+ Choose"} clear="— None" noun="specializations"
        disabled={save.busy} fav={favs.of("specialization")} manage={manage}
        choices={fs.options("specialization", current || null).map((o) => ({ value: o.key, label: o.label }))}
        onPick={(v) => { if (v !== current) save.mutate({ specialization: v || null }); }} />
      {save.status}
    </div>
  );
}

/** IT skills or certifications: chips with ×, and "+ Add" with the Settings list. */
export function QuickWords({ candidate, canEdit, field }: { candidate: Candidate; canEdit: boolean; field: "skills" | "certifications" }) {
  const fs = useFieldSettings();
  const favs = useFavourites();
  const save = useQuickSave(candidate);
  const words = candidate[field];
  const list = field === "skills" ? "it_skill" as const : "certification" as const;
  const manage = useWordListManager(list, favs.of(field === "skills" ? "itSkill" : "certification"));
  if (!canEdit) return <Chips labels={words} />;
  const meta = field === "skills"
    ? { list: "it_skill" as const, fav: favs.of("itSkill"), noun: "IT skills", one: "IT skill" }
    : { list: "certification" as const, fav: favs.of("certification"), noun: "certifications", one: "certification" };
  const has = (w: string) => words.some((x) => x.toLowerCase() === w.toLowerCase());
  const left = fs.options(meta.list).map((o) => o.label).filter((w) => !has(w));
  return (
    <div className="quick-edit">
      {words.map((w) => (
        <span key={w} className="chip">{w}
          <button type="button" className="chip-x" aria-label={`Remove ${w}`} disabled={save.busy}
            onClick={() => { save.mutate({ [field]: words.filter((x) => x !== w) }); }}>×</button>
        </span>
      ))}
      {(left.length > 0 || manage) && (
        <span className="fav-dropdown-add">
          <FavDropdown inline aria-label={`Add ${meta.one === "IT skill" ? "an" : "a"} ${meta.one}`} text="+ Add" noun={meta.noun} fav={meta.fav} disabled={save.busy} manage={manage}
            choices={left.map((w) => ({ value: w, label: w }))} onPick={(v) => { if (v && !has(v)) save.mutate({ [field]: [...words, v] }); }} />
        </span>
      )}
      {words.length === 0 && left.length === 0 && "—"}
      {save.status}
    </div>
  );
}

/** Industries: chips (the current one marked) with ×, and "+ Add" searching the whole catalogue. */
export function QuickIndustries({ candidate, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const industries = useIndustries();
  const favs = useFavourites();
  const save = useQuickSave(candidate);
  const manage = useIndustryManager();
  const data = industries.data;
  if (!canEdit || !data) return <IndustrySummary value={candidate.industrySelections} legacy={candidate.industry} />;
  const items = data.items;
  const chosen = candidate.industrySelections?.ids ?? [];
  const primary = candidate.industrySelections?.primaryId;
  const active = items.filter((i) => !i.retired && !items.find((p) => p.id === i.parentId)?.retired);
  // By name as read: each industry, then its subindustries ("Financial Services / FSI – Banking").
  const choices = active.filter((i) => !chosen.includes(i.id))
    .map((i) => ({ value: i.id, label: categoryLabel(i, items) }))
    .sort((a, b) => a.label.localeCompare(b.label));
  const rootLabel = (id?: string) => {
    const item = items.find((i) => i.id === id);
    const root: Category | undefined = item?.parentId ? items.find((i) => i.id === item.parentId) : item;
    return root?.label ?? null;
  };
  const change = (ids: string[], primaryId?: string) => {
    const p = primaryId && ids.includes(primaryId) ? primaryId : ids[0];
    save.mutate({ industrySelections: { catalogueVersion: data.version, ids, ...(p ? { primaryId: p } : {}) }, industry: rootLabel(p) });
  };
  return (
    <div className="quick-edit">
      {chosen.length === 0 && candidate.industry && <span className="muted">{candidate.industry}</span>}
      {chosen.map((id) => {
        const item = items.find((i) => i.id === id);
        const label = item ? categoryLabel(item, items) : "Removed category";
        return (
          <span key={id} className={`chip${id === primary ? " current" : ""}`} title={id === primary ? "Current industry" : undefined}>
            {label}{id === primary && chosen.length > 1 && <small>Current</small>}
            <button type="button" className="chip-x" aria-label={`Remove ${label}`} disabled={save.busy}
              onClick={() => { change(chosen.filter((x) => x !== id), primary); }}>×</button>
          </span>
        );
      })}
      <span className="fav-dropdown-add">
        <FavDropdown inline aria-label="Add an industry" text="+ Add" noun="industries" fav={favs.of("industry")} disabled={save.busy} manage={manage}
          choices={choices} onPick={(v) => { if (v && !chosen.includes(v)) change([...chosen, v], primary ?? v); }} />
      </span>
      {save.status}
    </div>
  );
}
