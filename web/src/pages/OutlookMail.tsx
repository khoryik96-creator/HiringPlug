import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type CSSProperties, useEffect, useState, useSyncExternalStore } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { ClockIcon, FileIcon, IconActions, IconButton, IconLink, PowerIcon, RefreshIcon, StepsIcon } from "../components/ActionIcons";
import { ErrorMessage } from "../components/ErrorMessage";
import { Splitter, useStoredWidth } from "../components/Splitter";
import { OutlookATSComposer } from "../components/outlook/OutlookATSComposer";
import { OutlookCommandBar, useOutlookChange } from "../components/outlook/OutlookCommandBar";
import { OutlookFolderPane, OutlookMessageRow } from "../components/outlook/OutlookMessageList";
import type { OutlookProfile } from "../components/outlook/OutlookProfileMail";
import { OutlookDraftEditor } from "../components/outlook/OutlookDraftEditor";
import { OutlookScheduledList } from "../components/outlook/OutlookScheduledList";
import { OutlookReadingPane } from "../components/outlook/OutlookReadingPane";
import { OutlookRules } from "../components/outlook/OutlookRules";
import { messageState, useCvImports } from "../components/outlook/outlookCvImports";
import "./outlook-workspace.css";

type Folder = Schemas["OutlookMailFolder"];

function watchWidth(change: () => void) {
  window.addEventListener("resize", change);
  return () => { window.removeEventListener("resize", change); };
}

/** Where the person last was in their mail (folder, thread, message, open draft), kept in this browser. */
type Place = { folder: string; thread: string; selected: string; draft: string };
const placeKey = (userId?: string) => `hiringplug.outlook.place:${userId ?? ""}`;
export function loadPlace(userId?: string): Place | null {
  try {
    const v = JSON.parse(localStorage.getItem(placeKey(userId)) ?? "null") as Partial<Place> | null;
    if (!v || typeof v !== "object") return null;
    const text = (x: unknown) => (typeof x === "string" ? x : "");
    return { folder: text(v.folder), thread: text(v.thread), selected: text(v.selected), draft: text(v.draft) };
  } catch { return null; }
}
function savePlace(userId: string | undefined, place: Place) {
  try { localStorage.setItem(placeKey(userId), JSON.stringify(place)); } catch { /* private window: not kept */ }
}

/** The widest the message list may be: about half of what the folders leave, so the reading pane keeps room. */
export function listLimit(screen: number, folders: number): number {
  return Math.max(260, Math.min(760, Math.round((screen - folders - 80) * 0.55)));
}

export function OutlookMail({ canImportCv = false, permissions = [], userId, docked = false }: {
  canImportCv?: boolean; permissions?: Permission[];
  /** The signed-in person: the CVs they imported from their mail show on it. */
  userId?: string;
  /** In the side dock, beside another page: compact, and the address is that page's, not the mail's. */
  docked?: boolean;
}) {
  if (docked) return <OutlookWorkspace docked canImportCv={canImportCv} permissions={permissions} userId={userId} initialSearch="" initialThread="" />;
  const params = new URLSearchParams(window.location.hash.split("?")[1]);
  if (params.has("rules")) return <OutlookRules />;
  const candidateId = params.get("candidateId");
  const contactId = params.get("contactId");
  const profile = candidateId ? { candidateId } : contactId ? { contactId } : undefined;
  return <OutlookWorkspace key={params.toString()} profile={profile} canImportCv={canImportCv} permissions={permissions} userId={userId}
    initialSearch={params.get("search") ?? ""} initialThread={params.get("conversation") ?? ""}/>;
}

function OutlookWorkspace({ profile, canImportCv, initialSearch, initialThread, permissions, userId, docked = false }: {
  docked?: boolean;
  profile?: OutlookProfile;
  canImportCv: boolean;
  initialSearch: string;
  initialThread: string;
  permissions: Permission[];
  userId?: string;
}) {
  const client = useQueryClient();
  // Opened plainly (not for a profile, a search or a thread): back where they last were.
  const [last] = useState(() => (!profile && !initialSearch && !initialThread ? loadPlace(userId) : null));
  const [search, setSearch] = useState(initialSearch);
  const [searchInput, setSearchInput] = useState(initialSearch);
  const [linked, setLinked] = useState(!!profile && !initialSearch);
  const [folder, setFolder] = useState(last?.folder ?? "");
  const [thread, setThread] = useState(last?.thread || initialThread);
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string | undefined>(last?.selected || undefined);
  // The draft that was open: opened again once the mail shows it is still there.
  const [draftToReopen, setDraftToReopen] = useState(last?.draft ?? "");
  const [scheduled, setScheduled] = useState(false);
  const [sentNote, setSentNote] = useState<"send" | "schedule" | null>(null);
  const [atsComposer, setATSComposer] = useState(false);
  // A null message starts a new draft.
  // fresh: just created here (new, reply, forward), so the signature goes in.
  const [draft, setDraft] = useState<{ id: string | null; focusMessage: boolean; fresh: boolean }>();
  const openDraft = (id: string | null, focusMessage = false, fresh = id === null || focusMessage) => { setDraft({ id, focusMessage, fresh }); };

  const queryParams = {
    ...(linked ? profile : {}),
    folder: folder || undefined,
    search: search || undefined,
    conversation: thread || undefined,
    offset,
  };
  const result = useQuery({
    queryKey: ["outlook", "list", queryParams],
    queryFn: async () => unwrap(await api.GET("/v1/outlook/mail", { params: { query: queryParams } })),
    refetchInterval: (q) => q.state.data?.enabled ? (q.state.data.syncing ? 5000 : 25000) : false,
  });
  const sync = useMutation({
    mutationFn: async (disable: boolean) => {
      if (disable) {
        unwrap(await api.DELETE("/v1/outlook/mail"));
      } else {
        unwrap(await api.POST("/v1/outlook/mail"));
      }
      return disable;
    },
    onSuccess: async (disabled) => {
      if (disabled) {
        setSelected(undefined);
        client.removeQueries({ queryKey: ["outlook", "body"] });
      }
      await client.invalidateQueries({ queryKey: ["outlook"] });
    },
  });
  const data = result.data;
  if (draftToReopen && data) {
    setDraftToReopen("");
    if (data.items.some((m) => m.id === draftToReopen && m.isDraft)) setDraft({ id: draftToReopen, focusMessage: false, fresh: false });
  }
  useEffect(() => {
    if (profile || search) return; // a profile's or a search's mail is not where they work from
    savePlace(userId, { folder, thread, selected: selected ?? "", draft: draft?.id ?? "" });
  }, [profile, search, userId, folder, thread, selected, draft?.id]);
  const folders = data?.folders ?? [];
  const active = data?.items.find((item) => item.id === selected);
  const inbox = folders.find((item) => item.kind === "inbox")?.id;
  const activeFolder = folder || (!search && !thread && !linked ? inbox : undefined);

  function chooseFolder(item: Folder) {
    setFolder(item.id);
    setThread("");
    setSearch("");
    setSearchInput("");
    setLinked(false);
    setOffset(0);
    setSelected(undefined);
  }

  const change = useOutlookChange();
  // The folder pane and the message list are as wide as last dragged.
  const [foldersWidth, setFoldersWidth] = useStoredWidth("hiringplug.outlook.foldersWidth", 200, 140, 360);
  const [storedList, setListWidth] = useStoredWidth("hiringplug.outlook.listWidth", 380, 260, 760);
  const screen = useSyncExternalStore(watchWidth, () => window.innerWidth);
  const listMax = listLimit(screen, foldersWidth);
  const listWidth = Math.min(storedList, listMax); // as shown, so dragging back starts where the bar is
  const imports = useCvImports(userId);
  const items = data?.items ?? [];
  /** Opens a message; Outlook marks it read as it opens. */
  function open(id: string) {
    setSelected(id);
    const item = items.find((m) => m.id === id);
    if (item && !item.isRead && !item.isDraft) change.mutate({ id, change: { isRead: true } });
  }
  /** Moves a message (Delete is a move to Deleted Items), then shows the next one, as Outlook does. */
  function move(id: string, folderId: string) {
    const at = items.findIndex((m) => m.id === id);
    const next = items[at + 1] ?? items[at - 1];
    change.mutate({ id, change: { folderId } });
    if (id === selected) setSelected(next && !next.isDraft ? next.id : undefined);
  }

  const Root = docked ? "div" : "main";
  return <Root className={docked ? "outlook-page outlook-docked" : "outlook-page"}>
    <div className="row spread">
      {!docked && <h2>Outlook Mail</h2>}
      {docked && folders.length > 0 && <select className="outlook-folder-select" aria-label="Folder" value={activeFolder ?? ""}
        onChange={(e) => { const f = folders.find((x) => x.id === e.target.value); if (f) chooseFolder(f); }}>
        {!activeFolder && <option value="">{search ? "Search results" : "Messages"}</option>}
        {folders.map((f) => <option key={f.id} value={f.id}>{f.name}{f.unread > 0 ? ` (${String(f.unread)})` : ""}</option>)}
      </select>}
      <IconActions label="Outlook actions">
        {data?.enabled && <IconButton label="Write with HiringPlug" icon={<FileIcon />} onClick={() => { setATSComposer(true); }} />}
        <IconButton label={scheduled ? "Back to mail" : "Scheduled"} icon={<ClockIcon />} pressed={scheduled}
          onClick={() => { setScheduled(!scheduled); }} />
        {data?.enabled && <>
          {!docked && <IconLink label="Inbox rules" icon={<StepsIcon />} href="#/outlook?rules=1" />}
          <IconButton label="Refresh" icon={<RefreshIcon />} disabled={sync.isPending} onClick={() => { sync.mutate(false); }} />
          {!docked && <IconButton label="Turn off sync" icon={<PowerIcon />} danger disabled={sync.isPending} onClick={() => {
            if (window.confirm("Turn off Outlook sync? Pending sends will be cancelled and HiringPlug’s profile links removed. Mail and drafts stay in Outlook.")) sync.mutate(true);
          }} />}
        </>}
      </IconActions>
    </div>
    <ErrorMessage error={result.error ?? sync.error}/>
    {sentNote && <p className="muted outlook-sent-note" role="status">
      {sentNote === "send" ? "Sent." : "Scheduled: it is under Scheduled (the clock), where it can be changed until it goes."}
      {" "}<button type="button" className="link" onClick={() => { setSentNote(null); }}>OK</button>
    </p>}
    {result.isPending && <p className="muted">Loading Outlook Mail…</p>}
    {!scheduled && data && !data.connected && <section className="panel">
      <p>Connect your Microsoft mailbox to use Outlook Mail.</p>
      <a className="button" href="#/email-connection">Email connection</a>
    </section>}
    {!scheduled && data?.connected && !data.enabled && <section className="panel">
      <p>Sync your folders and recent messages. Only you can see your Outlook mail here.</p>
      <button className="primary" type="button" disabled={sync.isPending} onClick={() => { sync.mutate(false); }}>Enable sync</button>
    </section>}
    {!scheduled && data?.enabled && <>
      {data.error && <p className="warning" role="alert">{data.error}</p>}
      {data.syncing && <p className="muted" role="status">Syncing Outlook folders…</p>}
      <section className="panel outlook-workspace" style={{ "--outlook-folders": `${String(foldersWidth)}px`, "--outlook-list": `${String(listWidth)}px` } as CSSProperties}>
        {!docked && <>
          <OutlookFolderPane folders={folders} active={activeFolder} onChoose={chooseFolder} />
          <Splitter label="Folder pane width" value={foldersWidth} min={140} max={360} onChange={setFoldersWidth} />
        </>}
        <div className="outlook-content">
          <div className="outlook-topbar">
          <OutlookCommandBar message={active} folders={folders} busy={change.isPending}
            canCompose={folders.some((item) => item.kind === "drafts")} onNew={() => { openDraft(null); }}
            onMove={(folderId) => { if (active) move(active.id, folderId); }}
            onRead={(isRead) => { if (active) change.mutate({ id: active.id, change: { isRead } }); }} />
          <form className="row outlook-search" role="search" onSubmit={(e) => { e.preventDefault(); setSearch(searchInput.trim()); setFolder(""); setThread(""); setLinked(false); setOffset(0); setSelected(undefined); }}>
            <input type="search" aria-label="Search Outlook mail" placeholder="Search by subject, name or email address" maxLength={254} value={searchInput} onChange={(e) => { setSearchInput(e.target.value); }}/>
            <button type="submit">Search</button>
            {profile && <button type="button" aria-pressed={linked} onClick={() => {
              setLinked(true); setThread(""); setFolder(""); setSearch(""); setSearchInput(""); setOffset(0); setSelected(undefined);
            }}>Linked to profile</button>}
            {thread && <button type="button" onClick={() => { setThread(""); setOffset(0); }}>Back to messages</button>}
          </form>
          </div>
          <ErrorMessage error={change.error} />
          <div className="outlook-columns">
            <div className="outlook-messages" aria-label="Messages">
              {data.items.length === 0 && <p className="muted">No messages to display.</p>}
              {data.items.map((item) => <OutlookMessageRow key={item.id} item={item} selected={item.id === selected} folders={folders}
                cvImport={messageState(imports, item.id)}
                onOpen={() => { if (item.isDraft) openDraft(item.id); else open(item.id); }}
                onMove={(folderId) => { move(item.id, folderId); }}
                onFlag={() => { change.mutate({ id: item.id, change: { flagged: !item.flagged } }); }} />)}
              {(offset > 0 || data.nextOffset != null) && <div className="row outlook-actions">
                <button type="button" disabled={!offset} onClick={() => { setOffset(Math.max(0, offset - 50)); }}>Previous</button>
                <button type="button" disabled={data.nextOffset == null} onClick={() => { setOffset(data.nextOffset ?? offset); }}>Next</button>
              </div>}
            </div>
            {!docked && <Splitter label="Message list width" value={listWidth} min={260} max={listMax} onChange={(w) => { setListWidth(Math.min(w, listMax)); }} />}
            {active ? <OutlookReadingPane key={active.id} message={active} folders={folders} profile={profile} linked={linked}
              canImportCv={canImportCv} userId={userId} onOpenDraft={openDraft}
              onThread={() => { setThread(active.conversation); setSearch(""); setFolder(""); setOffset(0); }} />
              : <p className="outlook-empty muted">Select a message to read it.</p>}
          </div>
        </div>
      </section>
    </>}
    {scheduled && <OutlookScheduledList onOpen={(id) => { openDraft(id); }}/>}
    {atsComposer && <OutlookATSComposer permissions={permissions} profile={profile} onClose={() => { setATSComposer(false); }} />}
    {draft && <OutlookDraftEditor key={draft.id ?? "new"} id={draft.id} focusMessage={draft.focusMessage} fresh={draft.fresh}
      permissions={permissions} about={profile ?? {}} onClose={(operation) => {
      setDraft(undefined);
      // Stay where you were (as Outlook does); a line says it went.
      if (operation === "send" || operation === "schedule") setSentNote(operation);
    }}/>}
  </Root>;
}
