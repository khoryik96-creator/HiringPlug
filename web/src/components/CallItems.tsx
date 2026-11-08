// What to ask on a call (Settings > Screening call or Reference check), a box
// per item, and the note they make: each label before its answer
// ("RFL: better prospects"), then any other notes.
import { Field } from "./Field";
import { useFieldSettings } from "./useFieldSettings";

export type CallList = "screening_call" | "reference_check";
export type CallAnswers = Record<string, string>;

/** The call's note: "RFL: ..." for each item answered, in the list's order, then the other notes. */
export function callNote(items: { key: string; label: string }[], answers: CallAnswers, other: string): string {
  const lines = items.flatMap((i) => {
    const answer = (answers[i.key] ?? "").trim();
    return answer ? [`${i.label}: ${answer}`] : [];
  });
  return [lines.join("\n"), other.trim()].filter(Boolean).join("\n\n");
}

/** The items to ask, in order (hidden ones left out). */
export function useCallItems(list: CallList) {
  return useFieldSettings().options(list);
}

export function CallItems({ list, answers, onChange }: { list: CallList; answers: CallAnswers; onChange: (a: CallAnswers) => void }) {
  const items = useCallItems(list);
  return (
    <div className="call-items full">
      {items.map((i) => (
        <Field key={i.key} label={i.label}>
          <textarea rows={2} value={answers[i.key] ?? ""} onChange={(e) => { onChange({ ...answers, [i.key]: e.target.value }); }} />
        </Field>
      ))}
    </div>
  );
}
