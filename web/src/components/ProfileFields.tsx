// Profile inputs shared by the profile form and the screening call.
import type React from "react";

import { WA_KIND } from "./candidateLook";
import type { CandidateFormState } from "./candidateFormState";
import { Field } from "./Field";
import { noticeKey, useFieldSettings } from "./useFieldSettings";

type Change = (patch: Partial<CandidateFormState>) => void;

/** The notice period, from the company's list (Settings). */
export function NoticeField({ form, onChange, label, error }: { form: CandidateFormState; onChange: Change; label: string; error?: string }) {
  const fs = useFieldSettings();
  const notice = form.noticeValue.trim() === "" ? "" : noticeKey(form.noticeValue.trim(), form.noticeUnit);
  return (
    <Field label={label} error={error}>
      <select value={notice} onChange={(e) => {
        const [value = "", unit = "month"] = e.target.value.split("_");
        onChange({ noticeValue: value, noticeUnit: unit as CandidateFormState["noticeUnit"] });
      }}>
        <option value="">—</option>
        {fs.options("notice_period", notice).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
      </select>
    </Field>
  );
}

/** Work authorization as one row of choices (the pass an expat holds included), coloured like the candidate card. */
const WA_CHOICES: { key: string; label: string; color: string; status: CandidateFormState["waStatus"]; basis: CandidateFormState["waBasis"] }[] = [
  { key: "local", label: "Local", color: WA_KIND.local.color, status: "local", basis: "" },
  { key: "pr", label: "PR", color: WA_KIND.pr.color, status: "expat_eligible", basis: "permanent_resident" },
  { key: "rp_t", label: "RP-T", color: WA_KIND.expat.color, status: "expat_eligible", basis: "rp_t" },
  { key: "spouse_visa", label: "Spouse visa", color: WA_KIND.expat.color, status: "expat_eligible", basis: "spouse_visa" },
  { key: "other", label: "Other pass", color: WA_KIND.expat.color, status: "expat_eligible", basis: "other" },
  { key: "needs", label: "Needs pass", color: WA_KIND.needsPass.color, status: "requires_visa", basis: "" },
  { key: "unknown", label: "Not known", color: WA_KIND.unknown.color, status: "", basis: "" },
];

export function WorkAuthFields({ form, onChange, label, errors }: {
  form: CandidateFormState;
  onChange: Change;
  label: string;
  errors: { status?: string; basis?: string };
}) {
  const on = (c: (typeof WA_CHOICES)[number]) => c.status === form.waStatus && (c.status !== "expat_eligible" || c.basis === form.waBasis);
  const error = errors.status ?? errors.basis;
  const at = Math.max(0, WA_CHOICES.findIndex(on)); // the one reached with Tab; arrows move the choice (a radio group)
  const choose = (i: number, e?: React.KeyboardEvent<HTMLButtonElement>) => {
    const c = WA_CHOICES[(i + WA_CHOICES.length) % WA_CHOICES.length];
    if (!c) return;
    onChange({ waStatus: c.status, waBasis: c.basis });
    const group = e?.currentTarget.parentElement;
    (group?.children[WA_CHOICES.indexOf(c)] as HTMLElement | undefined)?.focus();
  };
  return (
    <div className="field full">
      <span>{label}</span>
      <div className="wa-console" role="radiogroup" aria-label={label}>
        {WA_CHOICES.map((c, i) => (
          <button key={c.key} type="button" role="radio" aria-checked={on(c)} className={on(c) ? "on" : undefined}
            style={{ "--wa": c.color } as React.CSSProperties} tabIndex={i === at ? 0 : -1}
            onKeyDown={(e) => {
              if (e.key === "ArrowRight" || e.key === "ArrowDown") { e.preventDefault(); choose(i + 1, e); }
              if (e.key === "ArrowLeft" || e.key === "ArrowUp") { e.preventDefault(); choose(i - 1, e); }
            }}
            onClick={() => { choose(i); }}>
            <i aria-hidden="true" />{c.label}
          </button>
        ))}
      </div>
      {error && <small className="error">{error}</small>}
    </div>
  );
}
