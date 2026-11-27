// A folder's icon in the folder list, as Outlook shows them: by the folder's
// kind (Inbox, Drafts...), and for well-known folders Outlook makes without a
// kind (Outbox, Snoozed, Conversation History) by their name; a plain folder
// for the person's own.
import type { ReactNode } from "react";

import type { Schemas } from "../../api/client";

const Svg = ({ children }: { children: ReactNode }) => (
  <svg className="outlook-folder-icon" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.7"
    strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{children}</svg>
);

const ICONS: Record<string, () => ReactNode> = {
  inbox: () => <Svg><path d="M4 13V6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v7" /><path d="M4 13v5a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5h-4.5l-1.5 2.5h-4L8.5 13Z" /></Svg>,
  drafts: () => <Svg><path d="M4 20h4L19 9l-4-4L4 16v4Z" /><path d="m13.5 6.5 4 4" /></Svg>,
  sentitems: () => <Svg><path d="M21 3 3 10.5l7 2.5 2.5 7Z" /><path d="m10 13 4.5-4.5" /></Svg>,
  deleteditems: () => <Svg><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" /></Svg>,
  junkemail: () => <Svg><circle cx="12" cy="12" r="8.5" /><path d="m6 6 12 12" /></Svg>,
  archive: () => <Svg><rect x="3.5" y="4" width="17" height="4.5" rx="1" /><path d="M5 8.5V19a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8.5M10 12.5h4" /></Svg>,
  outbox: () => <Svg><path d="M4 13v5a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5" /><path d="M12 15V4M8 8l4-4 4 4" /></Svg>,
  snoozed: () => <Svg><circle cx="12" cy="13" r="7.5" /><path d="M12 9.5V13l2.5 1.5M5 4 3 6M19 4l2 2" /></Svg>,
  conversationhistory: () => <Svg><path d="M5 5h14a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H10l-4 3.5V16H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Z" /></Svg>,
  folder: () => <Svg><path d="M3.5 7a1.5 1.5 0 0 1 1.5-1.5h4l2 2h8a1.5 1.5 0 0 1 1.5 1.5v8.5A1.5 1.5 0 0 1 19 19H5a1.5 1.5 0 0 1-1.5-1.5Z" /></Svg>,
};

/** Which icon a folder gets: its kind, else its well-known name, else a folder. */
export function folderIconKey(folder: Pick<Schemas["OutlookMailFolder"], "kind" | "name">): string {
  if (folder.kind && folder.kind in ICONS) return folder.kind;
  const named = folder.name.toLowerCase().replace(/\s+/g, "");
  return named in ICONS ? named : "folder";
}

export function FolderIcon({ folder }: { folder: Pick<Schemas["OutlookMailFolder"], "kind" | "name"> }) {
  return <>{(ICONS[folderIconKey(folder)] ?? ICONS.folder)?.()}</>;
}
