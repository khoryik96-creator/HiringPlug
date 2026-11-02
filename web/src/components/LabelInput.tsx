import { useId, useState } from "react";

/**
 * Tags or skills: shown as chips with a remove button; type one and press
 * Enter or a comma to add it. Suggestions (values already used) come from a
 * datalist, so everyone spells them the same way.
 */
export function LabelInput({ id, value, onChange, suggestions, placeholder, ...aria }: {
  id?: string;
  value: string[];
  onChange: (labels: string[]) => void;
  suggestions: string[];
  placeholder: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  const listId = useId();
  const [text, setText] = useState("");
  const add = (raw: string) => {
    const label = raw.trim().replace(/\s+/g, " ");
    if (label && !value.some((v) => v.toLowerCase() === label.toLowerCase())) onChange([...value, label]);
    setText("");
  };
  return (
    <div>
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
      <input
        id={id}
        {...aria}
        list={listId}
        placeholder={placeholder}
        value={text}
        onChange={(e) => {
          const v = e.target.value;
          if (v.endsWith(",")) add(v.slice(0, -1));
          else setText(v);
        }}
        onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); add(text); } }}
        onBlur={() => { if (text.trim()) add(text); }}
      />
      <datalist id={listId}>
        {suggestions.filter((s) => !value.includes(s)).slice(0, 200).map((s) => <option key={s} value={s} />)}
      </datalist>
    </div>
  );
}

/** Tags or skills as read-only chips. */
export function Chips({ labels }: { labels: string[] }) {
  if (labels.length === 0) return <>—</>;
  return <span className="chips">{labels.map((l) => <span key={l} className="chip">{l}</span>)}</span>;
}
