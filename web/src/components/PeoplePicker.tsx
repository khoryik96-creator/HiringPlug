// Whose numbers to show: yourself, one or more people, or the whole team.
// A dropdown of ticks that stays open while choosing; the choice is
// remembered on this computer.
import { useEffect, useRef, useState } from "react";

/** Everyone together, or some people by id. */
export type People = "team" | string[];

const STORE = "hyplug.dashboard.people";

/** The remembered choice (yourself when none, or when it can't be read). */
export function useRememberedPeople(meId: string | undefined): [People, (p: People) => void] {
  const [people, setPeople] = useState<People | null>(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(STORE) ?? "null") as unknown;
      if (saved === "team" || (Array.isArray(saved) && saved.length > 0 && saved.every((s) => typeof s === "string"))) return saved;
    } catch { /* fall back to yourself */ }
    return null;
  });
  const choose = (p: People) => {
    setPeople(p);
    try { localStorage.setItem(STORE, JSON.stringify(p)); } catch { /* not remembered */ }
  };
  return [people ?? (meId ? [meId] : "team"), choose];
}

/** What the button says: "Me", "Aina", "Aina, Wei", "3 people" or "Whole team". */
export function peopleLabel(people: People, names: Map<string, string>, meId?: string): string {
  if (people === "team") return "Whole team";
  if (people.length === 1 && people[0] === meId) return "Me";
  if (people.length > 2) return `${String(people.length)} people`;
  return people.map((id) => (id === meId ? "Me" : names.get(id) ?? "Someone")).join(", ");
}

export function PeoplePicker({ people, onChange, everyone, meId }: {
  people: People; onChange: (p: People) => void; everyone: { id: string; name: string }[]; meId?: string;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const outside = (e: PointerEvent) => { if (!root.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", outside);
    return () => { document.removeEventListener("pointerdown", outside); };
  }, [open]);
  const names = new Map(everyone.map((p) => [p.id, p.name]));
  const chosen = new Set(people === "team" ? [] : people);
  const toggle = (id: string) => {
    const next = new Set(chosen);
    if (next.has(id)) next.delete(id); else next.add(id);
    onChange(next.size === 0 ? "team" : [...next]);
  };
  // Yourself first, then by name.
  const listed = [...everyone].sort((a, b) => (a.id === meId ? -1 : b.id === meId ? 1 : a.name.localeCompare(b.name)));
  return (
    <div className="people-picker" ref={root}
      onKeyDown={(e) => { if (e.key === "Escape") { setOpen(false); root.current?.querySelector("button")?.focus(); } }}>
      <button type="button" className="people-picker-button" aria-haspopup="true" aria-expanded={open} onClick={() => { setOpen(!open); }}>
        <span className="muted">Showing</span> <strong>{peopleLabel(people, names, meId)}</strong> <span aria-hidden>▾</span>
      </button>
      {open && (
        <div className="people-picker-list" role="group" aria-label="Whose numbers">
          <label className="people-picker-team">
            <input type="checkbox" checked={people === "team"} onChange={() => { onChange(people === "team" && meId ? [meId] : "team"); }} />
            Whole team
          </label>
          {listed.map((p) => (
            <label key={p.id}>
              <input type="checkbox" checked={chosen.has(p.id)} onChange={() => { toggle(p.id); }} />
              {p.id === meId ? `${p.name} (me)` : p.name}
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
