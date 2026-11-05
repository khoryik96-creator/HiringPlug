// Profile inputs shared by the profile form and the screening call.
import { keysOf, WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";
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

/** Work authorization, and the pass an expat holds when it applies. */
export function WorkAuthFields({ form, onChange, label, errors }: {
  form: CandidateFormState;
  onChange: Change;
  label: string;
  errors: { status?: string; basis?: string };
}) {
  return (
    <>
      <Field label={label} error={errors.status}>
        <select value={form.waStatus} onChange={(e) => { onChange({ waStatus: e.target.value as CandidateFormState["waStatus"] }); }}>
          <option value="">—</option>
          {keysOf(WORK_AUTH_STATUS).map((s) => <option key={s} value={s}>{WORK_AUTH_STATUS[s]}</option>)}
        </select>
      </Field>
      {form.waStatus === "expat_eligible" && (
        <Field label="Pass held" error={errors.basis}>
          <select value={form.waBasis} onChange={(e) => { onChange({ waBasis: e.target.value as CandidateFormState["waBasis"] }); }}>
            <option value="">Choose…</option>
            {keysOf(WORK_AUTH_BASIS).map((b) => <option key={b} value={b}>{WORK_AUTH_BASIS[b]}</option>)}
          </select>
        </Field>
      )}
    </>
  );
}
