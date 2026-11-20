// Outlook's command bar for the selected message: New mail, then Delete,
// Archive and Read/Unread as compact icons, each named in a tooltip (on hover
// and keyboard focus). Flag and Move to sit on each message in the list.
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { ArchiveIcon, ComposeIcon, IconButton, IconSeparator, MailCheckIcon, MailIcon, TrashIcon } from "../ActionIcons";

type Message = Schemas["OutlookMessage"];
type Folder = Schemas["OutlookMailFolder"];
type Page = Schemas["OutlookMailPage"];

/** Changes a message in Outlook, showing it in the list at once. */
export function useOutlookChange() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, change }: { id: string; change: Schemas["OutlookMessageChange"] }) => {
      unwrap(await api.PATCH("/v1/outlook/messages/{messageId}", { params: { path: { messageId: id } }, body: change }));
    },
    onMutate: ({ id, change }) => {
      client.setQueriesData<Page>({ queryKey: ["outlook", "list"] }, (page) => page && {
        ...page,
        items: page.items
          .filter((m) => !(change.folderId && m.id === id))
          .map((m) => (m.id === id ? { ...m, ...(change.isRead === undefined ? {} : { isRead: change.isRead }), ...(change.flagged === undefined ? {} : { flagged: change.flagged }) } : m)),
      });
    },
    onSettled: () => client.invalidateQueries({ queryKey: ["outlook", "list"] }),
  });
}

export function OutlookCommandBar({ message, folders, canCompose, busy, onNew, onMove, onRead }: {
  message?: Message; folders: Folder[]; canCompose: boolean; busy: boolean;
  onNew: () => void; onMove: (folderId: string) => void; onRead: (read: boolean) => void;
}) {
  const kind = (k: string) => folders.find((f) => f.kind === k);
  const deleted = kind("deleteditems"), archive = kind("archive");
  const usable = !!message && !message.isDraft && !busy;
  const here = message?.folderId;
  return (
    <div className="outlook-commandbar" role="toolbar" aria-label="Message commands">
      <button type="button" className="outlook-command primary" disabled={!canCompose} onClick={onNew}><ComposeIcon /><span>New mail</span></button>
      <IconSeparator />
      <IconButton label="Delete" icon={<TrashIcon />} danger disabled={!usable || !deleted || here === deleted.id}
        onClick={() => { if (deleted) onMove(deleted.id); }} />
      <IconButton label="Archive" icon={<ArchiveIcon />} disabled={!usable || !archive || here === archive.id}
        onClick={() => { if (archive) onMove(archive.id); }} />
      <IconButton label={message?.isRead === false ? "Mark as read" : "Mark as unread"} icon={message?.isRead === false ? <MailCheckIcon /> : <MailIcon />}
        disabled={!usable} onClick={() => { if (message) onRead(!message.isRead); }} />
    </div>
  );
}
