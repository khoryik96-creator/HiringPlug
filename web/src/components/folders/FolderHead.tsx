// Above an open folder: its name, how many are in it, who it is shared with,
// and (for its owner) Share, Rename and Delete; those it is shared with may
// leave it.
import { useEffect, useRef, useState } from "react";

import { EditIcon, IconActions, IconButton, LeaveIcon, ShareIcon, TrashIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { Initials } from "../Initials";
import { FolderIcon } from "./FolderPanel";
import { type Folder, type FolderKind, plural, useFolderChanges, useUsers } from "./useFolders";

export function FolderHead({ kind, folder, meId, onGone }: {
  kind: FolderKind;
  folder: Folder;
  meId: string;
  /** The folder was deleted or left: the list goes back to everything. */
  onGone: () => void;
}) {
  const users = useUsers();
  const change = useFolderChanges(kind);
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(folder.name);
  const [sharing, setSharing] = useState(false);
  const nameOf = (id: string) => users.data?.find((u) => u.id === id)?.name ?? "Someone who left";
  const count = `${String(folder.itemCount)} ${plural(kind, folder.itemCount)}`;

  return (
    <div className="folder-head">
      <FolderIcon shared={!folder.mine || folder.sharedWith.length > 0} />
      {renaming ? (
        <form className="folder-rename" onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) change.rename.mutate({ folder, name: name.trim() }, { onSuccess: () => { setRenaming(false); } });
        }}>
          <input aria-label="Folder name" maxLength={100} autoFocus value={name} onChange={(e) => { setName(e.target.value); }}
            onKeyDown={(e) => { if (e.key === "Escape") setRenaming(false); }} />
          <button type="submit" className="pill-btn primary" disabled={!name.trim() || change.rename.isPending}>Save</button>
          <button type="button" className="link small" onClick={() => { setRenaming(false); }}>Cancel</button>
        </form>
      ) : (
        <div className="folder-title">
          <h3>{folder.name}</h3>
          <span className="muted small">
            {count} · {folder.mine
              ? (folder.sharedWith.length > 0 ? "yours · shared with" : "yours · only you see it")
              : `from ${nameOf(folder.ownerId)} · you can add and remove`}
          </span>
        </div>
      )}
      {folder.mine && folder.sharedWith.length > 0 && !renaming && (
        <span className="folder-people">{folder.sharedWith.map((id) => <Initials key={id} name={nameOf(id)} />)}</span>
      )}
      <span className="sp" />
      <IconActions label="Folder actions">
        {folder.mine ? (
          <>
            <IconButton label="Share folder" icon={<ShareIcon />} onClick={() => { setSharing(true); }} />
            {!renaming && <IconButton label="Rename folder" icon={<EditIcon />} onClick={() => { setName(folder.name); setRenaming(true); }} />}
            <IconButton label="Delete folder" danger icon={<TrashIcon />} disabled={change.remove.isPending} onClick={() => {
              if (window.confirm(`Delete the folder "${folder.name}"? The ${plural(kind)} in it stay as they are.`)) {
                change.remove.mutate(folder.id, { onSuccess: onGone });
              }
            }} />
          </>
        ) : (
          <IconButton label="Leave folder" danger icon={<LeaveIcon />} disabled={change.unshare.isPending} onClick={() => {
            if (window.confirm(`Leave "${folder.name}"? You will no longer see it; ${nameOf(folder.ownerId)} can share it again.`)) {
              change.unshare.mutate({ folderId: folder.id, userId: meId }, { onSuccess: onGone });
            }
          }} />
        )}
      </IconActions>
      <ErrorMessage error={change.rename.error ?? change.remove.error ?? change.unshare.error} />
      {sharing && <ShareFolder kind={kind} folder={folder} meId={meId} onClose={() => { setSharing(false); }} />}
    </div>
  );
}

/** Who a folder is shared with: add someone of the agency, or remove them. */
function ShareFolder({ kind, folder, meId, onClose }: { kind: FolderKind; folder: Folder; meId: string; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const users = useUsers();
  const change = useFolderChanges(kind);
  const [who, setWho] = useState("");
  useEffect(() => {
    const d = dialog.current;
    if (d && !d.open) {
      if (typeof d.showModal === "function") d.showModal();
      else d.setAttribute("open", "");
    }
  }, []);
  const others = users.data?.filter((u) => u.id !== meId && !folder.sharedWith.includes(u.id)) ?? [];
  const nameOf = (id: string) => users.data?.find((u) => u.id === id)?.name ?? "Someone who left";
  const busy = change.share.isPending || change.unshare.isPending;

  return (
    <dialog ref={dialog} className="share-folder" aria-label={`Share ${folder.name}`} onClose={onClose} onCancel={onClose}>
      <h3>Share “{folder.name}”</h3>
      <p className="muted small">
        People you share with see the folder and can add or remove {plural(kind)}. Only you can rename, delete or share it.
      </p>
      <form className="row" onSubmit={(e) => {
        e.preventDefault();
        if (who) change.share.mutate({ folderId: folder.id, userId: who }, { onSuccess: () => { setWho(""); } });
      }}>
        <select aria-label="Someone to share with" value={who} onChange={(e) => { setWho(e.target.value); }}>
          <option value="">{others.length > 0 ? "Choose someone of your agency" : "Everyone already has it"}</option>
          {others.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
        <button type="submit" className="primary" disabled={!who || busy}>Share</button>
      </form>
      <ul className="share-people">
        <li><Initials name={nameOf(meId)} /><span><strong>{nameOf(meId)} (you)</strong><small>Owner</small></span></li>
        {folder.sharedWith.map((id) => (
          <li key={id}>
            <Initials name={nameOf(id)} />
            <span><strong>{nameOf(id)}</strong><small>Can add and remove</small></span>
            <button type="button" className="pill-btn" disabled={busy} aria-label={`Remove ${nameOf(id)}`}
              onClick={() => { change.unshare.mutate({ folderId: folder.id, userId: id }); }}>Remove</button>
          </li>
        ))}
      </ul>
      <ErrorMessage error={change.share.error ?? change.unshare.error ?? users.error} />
      <div className="row end"><button type="button" onClick={onClose}>Done</button></div>
    </dialog>
  );
}
