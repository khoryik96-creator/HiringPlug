import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type CSSProperties, useState } from "react";

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

export function OutlookMail({ canImportCv = false, permissions = [], userId }: {
  canImportCv?: boolean; permissions?: Permission[];
  /** The signed-in person: the CVs they imported from their mail show on it. */
  userId?: string;
}) {
  const params = new URLSearchParams(window.location.hash.split("?")[1]);
  if (params.has("rules")) return <OutlookRules />;
  const candidateId = params.get("candidateId");
  const contactId = params.get("contactId");
  const profile = candidateId ? { candidateId } : contactId ? { contactId } : undefined;
  return <OutlookWorkspace key={params.toString()} profile={profile} canImportCv={canImportCv} permissions={permissions} userId={userId}
    initialSearch={params.get("search") ?? ""} initialThread={params.get("conversation") ?? ""}/>;
}

function OutlookWorkspace({ profile, canImportCv, initialSearch, initialThread, permissions, userId }: {
  profile?: OutlookProfile;
  canImportCv: boolean;
  initialSearch: string;
  initialThread: string;
  permissions: Permission[];
  userId?: string;
}) {
  const client = useQueryClient();
  const [search, setSearch] = useState(initialSearch);
  const [searchInput, setSearchInput] = useState(initialSearch);
  const [linked, setLinked] = useState(!!profile && !initialSearch);
  const [folder, setFolder] = useState("");
  const [thread, setThread] = useState(initialThread);
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string>();
  const [scheduled, setScheduled] = useState(false);
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
  const [listWidth, setListWidth] = useStoredWidth("hiringplug.outlook.listWidth", 380, 260, 760);
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

  return <main className="outlook-page">
    <div className="row spread">
      <h2>Outlook Mail</h2>
      <IconActions label="Outlook actions">
        {data?.enabled && <IconButton label="Write with HiringPlug" icon={<FileIcon />} onClick={() => { setATSComposer(true); }} />}
        <IconButton label={scheduled ? "Back to mail" : "Scheduled"} icon={<ClockIcon />} pressed={scheduled}
          onClick={() => { setScheduled(!scheduled); }} />
        {data?.enabled && <>
          <IconLink label="Inbox rules" icon={<StepsIcon />} href="#/outlook?rules=1" />
          <IconButton label="Refresh" icon={<RefreshIcon />} disabled={sync.isPending} onClick={() => { sync.mutate(false); }} />
          <IconButton label="Turn off sync" icon={<PowerIcon />} danger disabled={sync.isPending} onClick={() => {
            if (window.confirm("Turn off Outlook sync? Pending sends will be cancelled and HiringPlug’s profile links removed. Mail and drafts stay in Outlook.")) sync.mutate(true);
          }} />
        </>}
      </IconActions>
    </div>
    <ErrorMessage error={result.error ?? sync.error}/>
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
        <OutlookFolderPane folders={folders} active={activeFolder} onChoose={chooseFolder} />
        <Splitter label="Folder pane width" value={foldersWidth} min={140} max={360} onChange={setFoldersWidth} />
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
            <Splitter label="Message list width" value={listWidth} min={260} max={760} onChange={setListWidth} />
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
      if (operation === "send" || operation === "schedule") setScheduled(true);
    }}/>}
  </main>;
}
