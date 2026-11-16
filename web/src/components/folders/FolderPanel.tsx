// The folders of a list, as tabs above it: everything, the person's own
// folders, those shared with them, and making a new one. A tab opens its
// folder (the list then shows what is in it) and takes rows dragged onto it.
// A long row of tabs scrolls sideways.
import { useState } from "react";

import { ErrorMessage } from "../ErrorMessage";
import { carriesRows, droppedRows, MAX_AT_ONCE, type Folder, type FolderKind, plural, useFolderChanges, useUsers } from "./useFolders";

export function FolderPanel({ kind, folders, open, onOpen, error }: {
  kind: FolderKind;
  folders: Folder[] | undefined;
  /** The open folder's id (null: everything). */
  open: string | null;
  onOpen: (id: string | null) => void;
  error?: unknown;
}) {
  const users = useUsers();
  const change = useFolderChanges(kind);
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const [over, setOver] = useState<string | null>(null); // the folder rows are dragged over
  const [dropped, setDropped] = useState<{ folder: string; n: number } | null>(null);
  const [tooMany, setTooMany] = useState(false);
  const ownerName = (id: string) => users.data?.find((u) => u.id === id)?.name ?? "a colleague";
  const mine = folders?.filter((f) => f.mine) ?? [];
  const shared = folders?.filter((f) => !f.mine) ?? [];

  const make = () => {
    const n = name.trim();
    if (!n) return;
    change.create.mutate(n, { onSuccess: (f) => { setNaming(false); setName(""); onOpen(f.id); } });
  };
  const tab = (f: Folder) => (
    <button key={f.id} type="button" className={`folder-tab${f.id === open ? " on" : ""}${f.id === over ? " drop" : ""}`}
      aria-current={f.id === open ? "true" : undefined} title={f.mine ? `Drop ${plural(kind)} here to add them` : `Shared by ${ownerName(f.ownerId)}`}
      onClick={() => { onOpen(f.id); }}
      onDragOver={(e) => { if (carriesRows(e)) { e.preventDefault(); e.dataTransfer.dropEffect = "copy"; setOver(f.id); } }}
      onDragLeave={() => { setOver((o) => (o === f.id ? null : o)); }}
      onDrop={(e) => {
        e.preventDefault();
        setOver(null);
        const ids = droppedRows(e);
        setTooMany(ids.length > MAX_AT_ONCE);
        if (ids.length > 0 && ids.length <= MAX_AT_ONCE) change.add.mutate({ folderId: f.id, itemIds: ids }, {
          onSuccess: () => { setDropped({ folder: f.name, n: ids.length }); window.setTimeout(() => { setDropped(null); }, 4000); },
        });
      }}>
      {(!f.mine || f.sharedWith.length > 0) && <FolderIcon shared />}
      <span className="folder-name">{f.name}</span>
      {!f.mine && <span className="visually-hidden">from {ownerName(f.ownerId)}</span>}
      <span className="folder-count">{f.itemCount}</span>
    </button>
  );

  return (
    <nav className="folder-tabs" aria-label="Folders">
      <div className="folder-strip">
        <button type="button" className={`folder-tab all${open === null ? " on" : ""}`} aria-current={open === null ? "true" : undefined}
          onClick={() => { onOpen(null); }}>
          <span className="folder-name">All {plural(kind)}</span>
        </button>
        {mine.map(tab)}
        {shared.length > 0 && mine.length > 0 && <span className="folder-sep" aria-hidden="true" />}
        {shared.map(tab)}
        {naming ? (
          <form className="folder-new" onSubmit={(e) => { e.preventDefault(); make(); }}>
            <input aria-label="New folder name" placeholder="Folder name" maxLength={100} autoFocus value={name}
              onChange={(e) => { setName(e.target.value); }} onKeyDown={(e) => { if (e.key === "Escape") setNaming(false); }} />
            <button type="submit" className="pill-btn primary" disabled={!name.trim() || change.create.isPending}>Create</button>
            <button type="button" className="link small" onClick={() => { setNaming(false); setName(""); }}>Cancel</button>
          </form>
        ) : (
          <button type="button" className="folder-tab folder-add" onClick={() => { setNaming(true); }}>+ New folder</button>
        )}
        {folders && mine.length === 0 && shared.length === 0 && !naming && (
          <span className="muted small folder-hint">Keep {plural(kind)} together: make a folder, then drag rows onto it.</span>
        )}
      </div>
      {tooMany && <p role="alert" className="warning small folder-done">At most {MAX_AT_ONCE} at a time: choose fewer.</p>}
      {dropped && <p role="status" className="small folder-done">✓ {dropped.n} {plural(kind, dropped.n)} put in {dropped.folder}</p>}
      <ErrorMessage error={error ?? change.create.error ?? change.add.error} />
    </nav>
  );
}

/** A folder; one with people in it when shared. */
export function FolderIcon({ shared }: { shared: boolean }) {
  return (
    <svg className="folder-icon" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round">
      <path d="M3 7.5A1.5 1.5 0 0 1 4.5 6H9l2 2h8.5A1.5 1.5 0 0 1 21 9.5v8a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5z" />
      {shared && <><circle cx="12" cy="12.6" r="1.7" /><path d="M8.8 17c.5-1.5 1.7-2.3 3.2-2.3s2.7.8 3.2 2.3" /></>}
    </svg>
  );
}
