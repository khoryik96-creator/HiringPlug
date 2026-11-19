import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import type { OutlookProfile } from "../components/outlook/OutlookProfileMail";
import { OutlookReadingPane } from "../components/outlook/OutlookReadingPane";
import { OutlookRules } from "../components/outlook/OutlookRules";
import { formatWhen } from "../labels";
import "./outlook-workspace.css";

type Message = Schemas["OutlookMessage"];
type Folder = Schemas["OutlookMailFolder"];

export function OutlookMail({ canSaveCv = false, canImportCv = false }: { canSaveCv?: boolean; canImportCv?: boolean }) {
  const params = new URLSearchParams(window.location.hash.split("?")[1]);
  if (params.has("rules")) return <OutlookRules />;
  const candidateId = params.get("candidateId");
  const contactId = params.get("contactId");
  const profile = candidateId ? { candidateId } : contactId ? { contactId } : undefined;
  return <OutlookWorkspace key={params.toString()} profile={profile} canSaveCv={canSaveCv} canImportCv={canImportCv}
    initialSearch={params.get("search") ?? ""} initialThread={params.get("conversation") ?? ""}/>;
}

function OutlookWorkspace({ profile, canSaveCv, canImportCv, initialSearch, initialThread }: {
  profile?: OutlookProfile;
  canSaveCv: boolean;
  canImportCv: boolean;
  initialSearch: string;
  initialThread: string;
}) {
  const client = useQueryClient();
  const [search, setSearch] = useState(initialSearch);
  const [searchInput, setSearchInput] = useState(initialSearch);
  const [linked, setLinked] = useState(!!profile && !initialSearch);
  const [folder, setFolder] = useState("");
  const [thread, setThread] = useState(initialThread);
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string>();
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

  function subject(item: Message) {
    return item.subject || "(No subject)";
  }

  return <main className="outlook-page">
    <div className="row spread">
      <h2>Outlook Mail</h2>
      {data?.enabled && <div className="row outlook-actions">
        <a className="button" href="#/outlook?rules=1">Inbox rules</a>
        <button type="button" disabled={sync.isPending} onClick={() => { sync.mutate(false); }}>Refresh</button>
        <button type="button" disabled={sync.isPending} onClick={() => {
          if (window.confirm("Turn off Outlook sync and remove HiringPlug’s local index and profile links? Your Outlook mail stays in Outlook.")) sync.mutate(true);
        }}>Turn off sync</button>
      </div>}
    </div>
    <ErrorMessage error={result.error ?? sync.error}/>
    {result.isPending && <p className="muted">Loading Outlook Mail…</p>}
    {data && !data.connected && <section className="panel">
      <p>Connect your Microsoft mailbox to use Outlook Mail.</p>
      <a className="button" href="#/email-connection">Email connection</a>
    </section>}
    {data?.connected && !data.enabled && <section className="panel">
      <p>Sync your folders and recent messages. Only you can see your Outlook mail here.</p>
      <button className="primary" type="button" disabled={sync.isPending} onClick={() => { sync.mutate(false); }}>Enable sync</button>
    </section>}
    {data?.enabled && <>
      {data.error && <p className="warning" role="alert">{data.error}</p>}
      {data.syncing && <p className="muted" role="status">Syncing Outlook folders…</p>}
      <section className="panel outlook-workspace">
        <nav className="outlook-folders" aria-label="Outlook folders">
          {folders.map((item) => <button type="button" key={item.id} aria-current={activeFolder === item.id ? "page" : undefined} onClick={() => { chooseFolder(item); }}>{item.name}</button>)}
        </nav>
        <div className="outlook-content">
          <form className="row outlook-actions" onSubmit={(e) => { e.preventDefault(); setSearch(searchInput.trim()); setFolder(""); setThread(""); setLinked(false); setOffset(0); setSelected(undefined); }}>
            <input aria-label="Search Outlook mail" placeholder="Subject or email address" maxLength={254} value={searchInput} onChange={(e) => { setSearchInput(e.target.value); }}/>
            <button type="submit">Search</button>
            {profile && <button type="button" aria-pressed={linked} onClick={() => {
              setLinked(true); setThread(""); setFolder(""); setSearch(""); setSearchInput(""); setOffset(0); setSelected(undefined);
            }}>Linked to profile</button>}
            {thread && <button type="button" onClick={() => { setThread(""); setOffset(0); }}>Back to messages</button>}
          </form>
          <div className="outlook-columns">
            <div className="outlook-messages" aria-label="Messages">
              {data.items.length === 0 && <p className="muted">No messages to display.</p>}
              {data.items.map((item) => <button type="button" className="outlook-message" aria-pressed={item.id === selected} key={item.id}
                onClick={() => { setSelected(item.id); }}>
                <span>{!item.isRead && !item.isDraft && <span className="outlook-unread" aria-label="Unread"/>}{subject(item)}</span>
                <small>{item.sender || (item.isDraft ? "Draft" : "Outlook")} · {formatWhen(item.receivedAt)}</small>
              </button>)}
              {(offset > 0 || data.nextOffset != null) && <div className="row outlook-actions">
                <button type="button" disabled={!offset} onClick={() => { setOffset(Math.max(0, offset - 50)); }}>Previous</button>
                <button type="button" disabled={data.nextOffset == null} onClick={() => { setOffset(data.nextOffset ?? offset); }}>Next</button>
              </div>}
            </div>
            {active ? <OutlookReadingPane key={active.id} message={active} folders={folders} profile={profile} linked={linked}
              canSaveCv={canSaveCv} canImportCv={canImportCv}
              onThread={() => { setThread(active.conversation); setSearch(""); setFolder(""); setOffset(0); }} />
              : <p className="outlook-empty muted">Select a message to read it.</p>}
          </div>
        </div>
      </section>
    </>}
  </main>;
}
