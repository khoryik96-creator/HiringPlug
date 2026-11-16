// What a list page needs for folders: the open folder (kept on this device),
// the folder panel beside the list, and for the chosen rows "Add to folder"
// and "Remove from this folder".
import { type ReactNode, useEffect, useState } from "react";

import { ErrorMessage } from "../ErrorMessage";
import { AddToFolder } from "./AddToFolder";
import { FolderHead } from "./FolderHead";
import { FolderPanel } from "./FolderPanel";
import { dragRows, type FolderKind, keepOpenFolder, MAX_AT_ONCE, plural, readOpenFolder, useFolderChanges, useFolderList, useMemberships } from "./useFolders";

export interface FolderView {
  /** The person may use folders. */
  on: boolean;
  kind: FolderKind;
  /** The open folder's id, to filter the list (undefined: everything). */
  filter: string | undefined;
  /** False while a remembered folder is being looked up (the list waits). */
  ready: boolean;
  /** Folder id → its name. */
  names: ReadonlyMap<string, string>;
  /** Whether to show an "In folders" column (the person has folders). */
  hasFolders: boolean;
  /** Lets a row be dragged onto a folder: the chosen rows when it is one, else that row. */
  drag: (id: string, chosen: ReadonlySet<string>) => ReturnType<typeof dragRows> | undefined;
  panel: (tools?: ReactNode) => ReactNode;
  head: ReactNode;
  /** The PickBar's folder buttons for the chosen rows. */
  actions: (chosen: string[]) => ReactNode;
}

/**
 * The folders of a list. `unchoose` clears the chosen rows: after they are
 * taken out of the open folder, and when another folder opens (they are no
 * longer shown).
 */
export function useFolderView(kind: FolderKind, on: boolean, meId: string, unchoose: () => void): FolderView {
  const list = useFolderList(kind, on);
  const [open, setOpenState] = useState<string | null>(() => (on ? readOpenFolder(kind) : null));
  const setOpen = (id: string | null) => {
    if (id !== open) unchoose();
    setOpenState(id);
    keepOpenFolder(kind, id);
  };
  const folder = open ? list.data?.find((f) => f.id === open) : undefined;
  // A folder deleted, or no longer shared with them: the list shows everything
  // (folder is undefined), and this device forgets it.
  const gone = open !== null && list.data !== undefined && !folder;
  useEffect(() => { if (gone) keepOpenFolder(kind, null); }, [gone, kind]);
  const hasFolders = (list.data?.length ?? 0) > 0;
  const change = useFolderChanges(kind);
  const names = new Map(list.data?.map((f) => [f.id, f.name]));

  return {
    on, kind, hasFolders,
    filter: folder?.id,
    ready: !on || open === null || list.data !== undefined || list.isError,
    names,
    drag: (id, chosen) => (on ? dragRows(() => (chosen.has(id) ? [...chosen] : [id])) : undefined),
    panel: (tools) => on && <FolderPanel kind={kind} folders={list.data} open={folder?.id ?? null} onOpen={setOpen} error={list.error} tools={tools} />,
    head: on && folder && <FolderHead key={folder.id} kind={kind} folder={folder} meId={meId} onGone={() => { setOpen(null); }} />,
    actions: (chosen) => on && (chosen.length > MAX_AT_ONCE ? <span className="warning small">At most {MAX_AT_ONCE} at a time for folders.</span> : (
      <>
        <AddToFolder kind={kind} ids={chosen} onTakenOut={(id) => { if (id === folder?.id) unchoose(); }} />
        {folder && (
          <button type="button" disabled={change.take.isPending} title={`Takes them out of this folder only: the ${plural(kind)} themselves stay as they are`}
            onClick={() => { change.take.mutate({ folderId: folder.id, itemIds: chosen }, { onSuccess: unchoose }); }}>Remove from this folder</button>
        )}
        <ErrorMessage error={change.take.error} />
      </>
    )),
  };
}

/** For the rows shown: the names of the folders each is in (for an "In folders" column). */
export function useInFolders(view: FolderView, ids: string[]): (id: string) => string[] {
  const member = useMemberships(view.kind, ids, view.on && view.hasFolders);
  return (id) => (member.data.get(id) ?? []).map((f) => view.names.get(f) ?? "").filter(Boolean);
}

/** The folders a record is in, in a list's "In folders" column. */
export function InFolders({ names }: { names: string[] }) {
  return <span className="in-folders">{names.join(", ")}</span>;
}

/** The list under its folder tabs (or the list alone, without folders); `tools` end the tabs' row. */
export function WithFolders({ view, tools, children }: { view: FolderView; tools?: ReactNode; children: ReactNode }) {
  if (!view.on) return <>{tools && <div className="list-tools-row"><span className="list-tools">{tools}</span></div>}{children}</>;
  return (
    <div className="with-folders">
      {view.panel(tools)}
      <div className="folder-main">
        {view.head}
        {children}
      </div>
    </div>
  );
}
