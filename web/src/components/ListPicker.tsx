// Choices from one of the company's lists (Settings > Dropdowns), such as IT
// skills or certifications: those chosen as chips with a remove button, and a
// dropdown to add more. Words already on the record that are not (or no
// longer) in the list stay until removed.
import { useId } from "react";

export function ListPicker({ value, onChange, options, addLabel, ...aria }: {
  value: string[];
  onChange: (labels: string[]) => void;
  /** The list's words, in its order. */
  options: string[];
  /** The dropdown's first line, e.g. "Add an IT skill…". */
  addLabel: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
  id?: string;
}) {
  const fallback = useId();
  const has = (o: string) => value.some((v) => v.toLowerCase() === o.toLowerCase());
  const left = options.filter((o) => !has(o));
  return (
    <div className="list-picker">
      {value.length > 0 && (
        <div className="chips">
          {value.map((v) => (
            <span key={v} className="chip">
              {v}{" "}
              <button type="button" className="link" aria-label={`Remove ${v}`} onClick={() => { onChange(value.filter((x) => x !== v)); }}>×</button>
            </span>
          ))}
        </div>
      )}
      <select id={aria.id ?? fallback} aria-invalid={aria["aria-invalid"]} aria-describedby={aria["aria-describedby"]} value=""
        disabled={left.length === 0} onChange={(e) => { if (e.target.value) onChange([...value, e.target.value]); }}>
        <option value="">{left.length === 0 ? "All chosen" : addLabel}</option>
        {left.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </div>
  );
}
