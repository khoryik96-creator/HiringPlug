import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatWhen } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { OutlookAttachments } from "./OutlookAttachments";
import type { OutlookProfile } from "./OutlookProfileMail";

type Message = Schemas["OutlookMessage"];
type Folder = Schemas["OutlookMailFolder"];

export function OutlookReadingPane({ message, folders, profile, linked, onThread }: {
  message: Message;
  folders: Folder[];
  profile?: OutlookProfile;
  linked: boolean;
  onThread: () => void;
}) {
  const client = useQueryClient();
  const [reply, setReply] = useState("");
  const [mode, setMode] = useState<"reply" | "replyAll" | null>(null);
  const [destination, setDestination] = useState("");
  const [notice, setNotice] = useState("");
  const path = { messageId: message.id };
  const params = { path };
  const body = useQuery({
    queryKey: ["outlook", "body", message.id],
    queryFn: async () => unwrap(await api.GET("/v1/outlook/messages/{messageId}", { params })),
    gcTime: 0,
    retry: false,
  });
  const change = useMutation({
    mutationFn: async (value: Schemas["OutlookMessageChange"]) => {
      unwrap(await api.PATCH("/v1/outlook/messages/{messageId}", { params, body: value }));
    },
    onSuccess: async () => { setDestination(""); await client.invalidateQueries({ queryKey: ["outlook", "list"] }); },
  });
  const send = useMutation({
    mutationFn: async (saveDraft: boolean) => {
      const result = await api.POST("/v1/outlook/messages/{messageId}/reply", { params, body: { text: reply, mode: mode ?? "reply", saveDraft } });
      if (!result.response.ok) unwrap(result);
      return saveDraft;
    },
    onSuccess: (saved) => {
      setNotice(saved ? "Reply saved in Outlook Drafts." : "Outlook accepted your reply.");
      setMode(null);
      setReply("");
    },
  });
  const link = useMutation({
    mutationFn: async (remove: boolean) => {
      if (remove) {
        unwrap(await api.DELETE("/v1/outlook/messages/{messageId}/link", { params }));
      } else if (profile) {
        unwrap(await api.PUT("/v1/outlook/messages/{messageId}/link", { params, body: profile }));
      }
      return remove;
    },
    onSuccess: async (removed) => {
      setNotice(removed ? "Profile link removed." : "Conversation linked to this profile.");
      await client.invalidateQueries({ queryKey: ["outlook", "profile"] });
      await client.invalidateQueries({ queryKey: ["outlook", "list"] });
    },
  });
  const archive = folders.find((folder) => folder.kind === "archive");
  return <section className="outlook-reading" aria-label="Outlook message">
    <h3>{message.subject || "(No subject)"}</h3>
    <p className="outlook-envelope"><strong>{message.sender || "Draft"}</strong> · {formatWhen(message.receivedAt)}</p>
    <div className="row outlook-actions">
      <button type="button" onClick={onThread}>Conversation</button>
      {message.webUrl && <a href={message.webUrl} target="_blank" rel="noreferrer">Open in Outlook</a>}
      <button type="button" disabled={change.isPending} onClick={() => { change.mutate({ isRead: !message.isRead }); }}>
        Mark {message.isRead ? "unread" : "read"}
      </button>
      {archive && archive.id !== message.folderId && <button type="button" disabled={change.isPending} onClick={() => { change.mutate({ folderId: archive.id }); }}>Archive</button>}
    </div>
    <div className="row outlook-actions">
      <select aria-label="Destination folder" value={destination} onChange={(e) => { setDestination(e.target.value); }}>
        <option value="">Move to…</option>
        {folders.filter((folder) => folder.id !== message.folderId).map((folder) => <option key={folder.id} value={folder.id}>{folder.name}</option>)}
      </select>
      <button type="button" disabled={!destination || change.isPending} onClick={() => { change.mutate({ folderId: destination }); }}>Move</button>
      {profile && <>
        {linked
          ? <button type="button" disabled={link.isPending} onClick={() => { link.mutate(true); }}>Unlink from profile</button>
          : <button type="button" disabled={link.isPending} onClick={() => { link.mutate(false); }}>Link to {profile.candidateId ? "candidate" : "contact"}</button>}
      </>}
    </div>
    <ErrorMessage error={body.error ?? change.error ?? send.error ?? link.error}/>
    {notice && <p role="status">{notice}</p>}
    {body.isPending && <p className="muted">Opening message…</p>}
    {body.data && <p className="outlook-body">{body.data.text}</p>}
    {body.data?.truncated && <p className="muted">Open in Outlook to read the rest.</p>}
    {message.hasAttachments && <OutlookAttachments key={message.id} messageId={message.id} />}
    {!message.isDraft && mode === null && <div className="row">
      <button type="button" onClick={() => { setMode("reply"); setNotice(""); }}>Reply</button>
      <button type="button" onClick={() => { setMode("replyAll"); setNotice(""); }}>Reply all</button>
    </div>}
    {mode && <form className="outlook-reply" onSubmit={(e) => { e.preventDefault(); send.mutate(false); }}>
      <label className="field">{mode === "replyAll" ? "Reply to all recipients" : "Reply to sender"}
        <textarea rows={5} maxLength={10000} required value={reply} onChange={(e) => { setReply(e.target.value); }} />
      </label>
      <div className="row outlook-actions">
        <button className="primary" type="submit" disabled={!reply.trim() || send.isPending}>Send {mode === "replyAll" ? "reply all" : "reply"}</button>
        <button type="button" disabled={!reply.trim() || send.isPending} onClick={() => { send.mutate(true); }}>Save in Outlook Drafts</button>
        <button type="button" disabled={send.isPending} onClick={() => { setMode(null); send.reset(); }}>Close reply</button>
      </div>
    </form>}
  </section>;
}
