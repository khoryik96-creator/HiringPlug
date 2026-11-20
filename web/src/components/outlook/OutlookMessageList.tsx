// The folder pane and the message list, as Outlook shows them: folders with
// their unread counts (sub-folders indented), and per message the sender and
// subject, unread ones bold with a bar, and beside the date its flag and
// Move to.
import { useRef } from "react";

import type { Schemas } from "../../api/client";
import { FlagIcon, MoveFolderIcon, PaperclipIcon } from "../ActionIcons";
import type { CvImportState } from "./outlookCvImports";

type Message = Schemas["OutlookMessage"];
type Folder = Schemas["OutlookMailFolder"];

/** Outlook's dates in a list: a time today, "Yesterday", a weekday this week, else the date. */
export function listDate(iso: string, now = new Date()): string {
  const d = new Date(iso);
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const days = Math.round((day(now) - day(d)) / 86_400_000);
  if (days === 0) return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (days === 1) return "Yesterday";
  if (days > 1 && days < 7) return d.toLocaleDateString(undefined, { weekday: "short" });
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short", ...(d.getFullYear() === now.getFullYear() ? {} : { year: "numeric" }) });
}

/** Folders in Outlook's order, each child under its parent, with its depth. */
export function folderTree(folders: Folder[]): { folder: Folder; depth: number }[] {
  const ids = new Set(folders.map((f) => f.id));
  const out: { folder: Folder; depth: number }[] = [];
  const add = (parent: string | null, depth: number) => {
    for (const f of folders) {
      const p = f.parentId && ids.has(f.parentId) ? f.parentId : null;
      if (p === parent && depth < 8) {
        out.push({ folder: f, depth });
        add(f.id, depth + 1);
      }
    }
  };
  add(null, 0);
  return out;
}

export function OutlookFolderPane({ folders, active, onChoose }: { folders: Folder[]; active?: string; onChoose: (f: Folder) => void }) {
  return (
    <nav className="outlook-folders" aria-label="Outlook folders">
      {folderTree(folders).map(({ folder, depth }) => (
        <button type="button" key={folder.id} aria-current={active === folder.id ? "page" : undefined}
          style={depth ? { paddingLeft: `${String(0.75 + depth * 0.9)}rem` } : undefined}
          className={folder.unread > 0 ? "has-unread" : undefined} onClick={() => { onChoose(folder); }}>
          <span className="outlook-folder-name">{folder.name}</span>
          {folder.unread > 0 && <span className="outlook-count" aria-label={`${String(folder.unread)} unread`}>{folder.unread}</span>}
        </button>
      ))}
    </nav>
  );
}

/** "Move to": the folders a message can go to, in a menu under a folder icon. */
export function MoveMenu({ folders, current, disabled = false, onMove }: {
  folders: Folder[]; current?: string; disabled?: boolean; onMove: (folderId: string) => void;
}) {
  const menu = useRef<HTMLDetailsElement>(null);
  return (
    <details ref={menu} className="outlook-move">
      <summary className={`icon-action${disabled ? " disabled" : ""}`} aria-label="Move to" aria-disabled={disabled}
        data-tip={disabled ? undefined : "Move to"} title={disabled ? "Move to" : undefined} onClick={(e) => { if (disabled) e.preventDefault(); }}>
        <MoveFolderIcon />
      </summary>
      <div className="outlook-move-menu" role="menu" aria-label="Move to folder">
        {folderTree(folders).filter(({ folder }) => folder.id !== current && folder.kind !== "drafts").map(({ folder, depth }) => (
          <button type="button" role="menuitem" key={folder.id} style={depth ? { paddingLeft: `${String(0.8 + depth * 0.8)}rem` } : undefined}
            onClick={() => { menu.current?.removeAttribute("open"); onMove(folder.id); }}>{folder.name}</button>
        ))}
      </div>
    </details>
  );
}

const IMPORT_NOTE = { importing: "Importing a CV from this message", done: "CV imported from this message", failed: "CV import failed for this message" } as const;

export function OutlookMessageRow({ item, selected, folders, cvImport, onOpen, onMove, onFlag }: {
  item: Message; selected: boolean; folders: Folder[];
  /** How a CV imported from its attachments went: outlined orange, green or red. */
  cvImport?: CvImportState;
  onOpen: () => void; onMove: (folderId: string) => void; onFlag: () => void;
}) {
  const subject = item.subject || "(No subject)";
  const who = item.isDraft ? "[Draft]" : (item.senderName || item.sender || "Outlook");
  const unread = !item.isRead && !item.isDraft;
  return (
    <div className={`outlook-row${unread ? " unread" : ""}${selected ? " selected" : ""}${cvImport ? ` cv-${cvImport}` : ""}`}>
      <button type="button" className="outlook-message" aria-pressed={selected} onClick={onOpen}
        title={item.sender && item.senderName ? `${item.senderName} <${item.sender}>` : undefined}>
        <span className="outlook-from">{unread && <span className="visually-hidden">Unread: </span>}{who}
          {cvImport && <span className="visually-hidden"> ({IMPORT_NOTE[cvImport]})</span>}</span>
        <span className="outlook-message-subject">
          <span className="outlook-subject-text">{subject}</span>
          {item.hasAttachments && <span className="outlook-marks" title="Has attachments" aria-label="Has attachments"><PaperclipIcon /></span>}
        </span>
      </button>
      <div className="outlook-row-side">
        <span className="outlook-when">{listDate(item.receivedAt)}</span>
        {!item.isDraft && (
          <span className="outlook-row-tools">
            <button type="button" className={`icon-action outlook-flag${item.flagged ? " on" : ""}`} aria-label={item.flagged ? "Unflag message" : "Flag message"}
              data-tip={item.flagged ? "Unflag" : "Flag"} aria-pressed={item.flagged} onClick={onFlag}><FlagIcon /></button>
            <MoveMenu folders={folders} current={item.folderId} onMove={onMove} />
          </span>
        )}
      </div>
    </div>
  );
}
