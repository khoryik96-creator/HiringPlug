// "Add to folder ▾" for the chosen rows: a ticked folder holds all of them
// (ticking puts them in, unticking takes them out), and a name not yet used
// makes a new folder with them in it.
import { useEffect, useRef, useState } from "react";

import { ErrorMessage } from "../ErrorMessage";
import { type FolderKind, useFolderChanges, useFolderList, useMemberships, useUsers } from "./useFolders";

export function AddToFolder({ kind, ids, onTakenOut }: {
  kind: FolderKind;
  ids: string[];
  /** The chosen were taken out of a folder (the open one: they are no longer shown). */
  onTakenOut?: (folderId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [find, setFind] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const folders = useFolderList(kind);
  const users = useUsers();
  const member = useMemberships(kind, ids, open);
  const change = useFolderChanges(kind);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("mousedown", away);
    return () => { document.removeEventListener("mousedown", away); };
  }, [open]);

  const held = (folderId: string) => ids.filter((id) => member.data.get(id)?.includes(folderId)).length;
  const typed = find.trim();
  const shown = (folders.data ?? []).filter((f) => f.name.toLowerCase().includes(typed.toLowerCase()));
  const exact = folders.data?.some((f) => f.mine && f.name.toLowerCase() === typed.toLowerCase());
  const busy = change.add.isPending || change.take.isPending || change.create.isPending;
  const ownerName = (id: string) => users.data?.find((u) => u.id === id)?.name;
  const makeWithThem = () => {
    change.create.mutate(typed, { onSuccess: (f) => { setFind(""); change.add.mutate({ folderId: f.id, itemIds: ids }); } });
  };

  return (
    <div className="add-to-folder" ref={box} onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}>
      <button type="button" className="primary" aria-expanded={open} aria-haspopup="true" onClick={() => { setOpen(!open); }}>
        Add to folder ▾
      </button>
      {open && (
        <div className="folder-menu" role="group" aria-label="Folders for the chosen">
          <input aria-label="Find or name a new folder" placeholder="Find or name a new folder" autoFocus value={find}
            onChange={(e) => { setFind(e.target.value); }}
            onKeyDown={(e) => {
              if (e.key === "Enter" && typed && !exact && !busy) {
                e.preventDefault();
                makeWithThem();
              }
            }} />
          <ul>
            {shown.map((f) => {
              const n = held(f.id);
              const all = n === ids.length && n > 0;
              return (
                <li key={f.id}>
                  <label>
                    <input type="checkbox" checked={all} disabled={busy || member.isPending}
                      ref={(el) => { if (el) el.indeterminate = n > 0 && !all; }}
                      onChange={() => {
                        if (all) change.take.mutate({ folderId: f.id, itemIds: ids }, { onSuccess: () => { onTakenOut?.(f.id); } });
                        else change.add.mutate({ folderId: f.id, itemIds: ids });
                      }} />
                    <span className="folder-name">{f.name}{!f.mine && <small> · {ownerName(f.ownerId) ?? "shared"}</small>}</span>
                    <span className="folder-count">{f.itemCount}</span>
                  </label>
                </li>
              );
            })}
            {folders.data && shown.length === 0 && !typed && <li className="muted small">No folders yet: type a name to make one.</li>}
          </ul>
          <div className="folder-menu-foot">
            {typed && !exact ? (
              <button type="button" className="link small" disabled={busy} onClick={makeWithThem}>+ New folder “{typed}”</button>
            ) : <span className="muted small">Tick a folder to put them in</span>}
            <button type="button" className="pill-btn primary" onClick={() => { setOpen(false); }}>Done</button>
          </div>
          <ErrorMessage error={change.add.error ?? change.take.error ?? change.create.error ?? member.error} />
        </div>
      )}
    </div>
  );
}
