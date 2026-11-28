// Choices from one of the company's lists (Settings > Dropdowns), such as IT
// skills or certifications: those chosen as chips with a remove button, and a
// dropdown to add more (the person's favourites on top, starred with ☆).
// Words already on the record that are not (or no longer) in the list stay
// until removed.
import { FavDropdown } from "./FavDropdown";
import type { Fav } from "./favourites";
import type { ListManager } from "./listManagers";

export function ListPicker({ value, onChange, options, addLabel, noun, fav, manage, ...aria }: {
  value: string[];
  onChange: (labels: string[]) => void;
  /** The list's words, in its order. */
  options: string[];
  /** The dropdown's text, e.g. "Add an IT skill…". */
  addLabel: string;
  /** The list's name, plural: "IT skills". */
  noun: string;
  fav?: Fav;
  /** Adding, renaming and removing the list's words (only for those who manage it). */
  manage?: ListManager;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
  id?: string;
}) {
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
      <FavDropdown id={aria.id} aria-invalid={aria["aria-invalid"]} aria-describedby={aria["aria-describedby"]}
        text={left.length === 0 && options.length > 0 ? "All chosen" : addLabel} disabled={left.length === 0 && !manage}
        noun={noun} fav={fav} manage={manage} choices={left.map((o) => ({ value: o, label: o }))} onPick={(v) => { if (v && !has(v)) onChange([...value, v]); }} />
    </div>
  );
}
