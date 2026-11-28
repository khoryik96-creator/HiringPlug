// A dropdown with a search box whose items can be starred: the person's
// favourites sit on top, the rest below, and typing finds any item. Used for
// adding an IT skill or certification and choosing a specialization. Those
// who manage the list (owners and admins) also add a word typed, rename one
// (✎) or remove one (×) right here.
import { type ReactNode, useEffect, useId, useRef, useState } from "react";

import { ErrorMessage } from "./ErrorMessage";
import { type Fav, frozen, splitFavourites, Star, StarFailed } from "./favourites";
import type { ListManager } from "./listManagers";

export type FavChoice = { value: string; label: string };

export function FavDropdown({ text, choices, onPick, fav, noun, clear, manage, disabled = false, inline = false, ...aria }: {
  /** What the closed dropdown shows: the choice made, or e.g. "Add an IT skill…". */
  text: ReactNode;
  choices: FavChoice[];
  onPick: (value: string) => void;
  fav?: Fav;
  /** The list's name, plural: "IT skills". */
  noun: string;
  /** A first line that clears the choice (for a single choice), e.g. "—". */
  clear?: string;
  /** Adding, renaming and removing the list's items (only for those who manage it). */
  manage?: ListManager;
  disabled?: boolean;
  /** Small and borderless, for editing in place (a profile's Details). */
  inline?: boolean;
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
  "aria-label"?: string;
}) {
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  // The order when opened: a star clicked now shows at once, but the item
  // moves to the top only the next time (so nothing jumps under the mouse).
  const [order, setOrder] = useState<readonly string[]>([]);
  const box = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const fallback = useId();
  const menuId = `${aria.id ?? fallback}-menu`;
  // Renaming or about to remove an item (its value), and a change being saved.
  const [editing, setEditing] = useState<{ value: string; name: string } | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<unknown>(null);
  const settle = () => { setEditing(null); setRemoving(null); setFailed(null); };
  // Closed (by a pick, Escape or a click elsewhere), it starts afresh next time.
  const close = () => { setOpen(false); setFilter(""); settle(); };
  const run = async (change: () => Promise<void>) => {
    setBusy(true);
    setFailed(null);
    try {
      await change();
      setEditing(null);
      setRemoving(null);
    } catch (e) {
      setFailed(e);
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) { setOpen(false); setFilter(""); settle(); } };
    document.addEventListener("mousedown", away);
    return () => { document.removeEventListener("mousedown", away); };
  }, [open]);
  const pick = (v: string) => { onPick(v); close(); };
  const words = filter.trim().toLowerCase();
  const shown = choices.filter((c) => !words || c.label.toLowerCase().includes(words));
  const { top, rest } = splitFavourites(shown, (c) => c.value, frozen(fav, order));
  const typed = filter.trim().replace(/\s+/g, " ");
  const canAdd = manage && typed && !editing && !removing && !choices.some((c) => c.label.toLowerCase() === typed.toLowerCase());
  const row = (c: FavChoice) => {
    if (manage && editing?.value === c.value) {
      return (
        <li key={c.value} className="fav-row fav-editing">
          <form className="fav-edit" onSubmit={(e) => { e.preventDefault(); if (editing.name.trim()) void run(() => manage.rename(c.value, editing.name)); }}>
            <input aria-label={`New name for ${c.label}`} value={editing.name} autoFocus disabled={busy}
              onChange={(e) => { setEditing({ value: c.value, name: e.target.value }); }}
              onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); settle(); } }} />
            <button type="submit" className="small" disabled={busy || !editing.name.trim()}>Save</button>
            <button type="button" className="link" disabled={busy} onClick={settle}>Cancel</button>
          </form>
        </li>
      );
    }
    if (manage && removing === c.value) {
      return (
        <li key={c.value} className="fav-row fav-confirm" role="group" aria-label={`Remove ${c.label}`}>
          <span>Remove <strong>{c.label}</strong> from the list? <span className="muted">{manage.removeNote}</span></span>
          <span className="row">
            <button type="button" className="small danger" disabled={busy} onClick={() => { void run(() => manage.remove(c.value)); }}>Remove</button>
            <button type="button" className="link" disabled={busy} autoFocus onClick={settle}>Cancel</button>
          </span>
        </li>
      );
    }
    return (
      <li key={c.value} className="fav-row">
        <button type="button" className="fav-pick" onClick={() => { pick(c.value); }}>{c.label}</button>
        {fav && <Star fav={fav} value={c.value} label={c.label} />}
        {manage && (
          <>
            <button type="button" className="fav-tool" aria-label={`Rename ${c.label}`} title="Rename for everyone"
              onClick={() => { setFailed(null); setRemoving(null); setEditing({ value: c.value, name: manage.nameOf?.(c.value) ?? c.label }); }}>✎</button>
            <button type="button" className="fav-tool remove" aria-label={`Remove ${c.label} from the list`} title="Remove from the list"
              onClick={() => { setFailed(null); setEditing(null); setRemoving(c.value); }}>×</button>
          </>
        )}
      </li>
    );
  };
  return (
    <div className={`fav-dropdown${inline ? " inline" : ""}`} ref={box} onKeyDown={(e) => { if (e.key === "Escape" && open) { e.stopPropagation(); close(); button.current?.focus(); } }}>
      <button ref={button} type="button" id={aria.id ?? fallback} className="fav-button" disabled={disabled} aria-haspopup="true" aria-expanded={open}
        aria-controls={open ? menuId : undefined} aria-invalid={aria["aria-invalid"]} aria-describedby={aria["aria-describedby"]} aria-label={aria["aria-label"]}
        onClick={() => { if (open) close(); else { setOrder(fav?.list ?? []); setOpen(true); } }}>
        <span>{text}</span><span aria-hidden="true" className="multi-caret">▾</span>
      </button>
      {open && (
        <div className="fav-menu" id={menuId}>
          <input className="multi-filter" type="search" placeholder={`Type to find any of the ${noun}…`} aria-label={`Find ${noun}`}
            value={filter} autoFocus onChange={(e) => { setFilter(e.target.value); }} />
          <div className="fav-options">
            {clear !== undefined && !words && <ul><li className="fav-row"><button type="button" className="fav-pick muted" onClick={() => { pick(""); }}>{clear}</button></li></ul>}
            {top.length > 0 && <><p className="fav-group">★ Your favourites</p><ul aria-label="Your favourites">{top.map(row)}</ul></>}
            {rest.length > 0 && <>{top.length > 0 && <p className="fav-group">All {noun}</p>}<ul aria-label={`All ${noun}`}>{rest.map(row)}</ul></>}
            {canAdd && (
              <ul><li className="fav-row"><button type="button" className="fav-pick fav-add" disabled={busy}
                onClick={() => { void run(async () => { const v = await manage.add(typed); if (v) pick(v); }); }}>+ Add “{typed}” to the list</button></li></ul>
            )}
            {shown.length === 0 && !canAdd && <p className="muted small fav-none">{choices.length === 0 ? `No ${noun} to add.` : "None match."}</p>}
          </div>
          {manage && <ErrorMessage error={failed} />}
          <StarFailed fav={fav} />
          {fav?.ready && fav.list.length === 0 && choices.length > 3 && <p className="muted small fav-tip">Tip: click ☆ to keep one at the top, just for you.</p>}
        </div>
      )}
    </div>
  );
}
