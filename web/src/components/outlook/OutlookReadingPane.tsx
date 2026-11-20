import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatWhen } from "../../labels";
import { ArchiveIcon, ConversationIcon, ForwardIcon, IconActions, IconButton, IconLink, MailCheckIcon, MailIcon, MoveFolderIcon, OutlookIcon, ReplyAllIcon, ReplyIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { OutlookAttachments } from "./OutlookAttachments";
import { OutlookForward } from "./OutlookForward";
import type { OutlookProfile } from "./OutlookProfileMail";

type Message = Schemas["OutlookMessage"];
type Folder = Schemas["OutlookMailFolder"];

export function OutlookReadingPane({ message, folders, profile, linked, canSaveCv, canImportCv, onThread, onOpenDraft }: {
  message: Message;
  folders: Folder[];
  profile?: OutlookProfile;
  linked: boolean;
  canSaveCv: boolean;
  canImportCv: boolean;
  onThread: () => void;
  onOpenDraft: (id: string, focusMessage?: boolean) => void;
}) {
  const client = useQueryClient();
  const [reply, setReply] = useState("");
  const [mode, setMode] = useState<"reply" | "replyAll" | "forward" | null>(null);
  const [destination, setDestination] = useState("");
  const [notice, setNotice] = useState("");
  const draftsReady = folders.some((folder) => folder.kind === "drafts");
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
      const result = await api.POST("/v1/outlook/messages/{messageId}/reply", { params, body: { text: reply, mode: mode === "replyAll" ? "replyAll" : "reply", saveDraft } });
      if (!result.response.ok) unwrap(result);
      return saveDraft;
    },
    onSuccess: (saved) => {
      setNotice(saved ? "Reply saved in Outlook Drafts." : "Outlook accepted your reply.");
      setMode(null);
      setReply("");
    },
  });
  const draftReply = useMutation({
    mutationFn: async (replyAll: boolean) => unwrap(await api.POST("/v1/outlook/messages/{messageId}/reply-draft", {
      params, body: { replyAll },
    })),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ["outlook", "list"] }); },
  });
  const startReply = (replyAll: boolean) => {
    setNotice("");
    if (!draftsReady) {
      setMode(replyAll ? "replyAll" : "reply");
      return;
    }
    draftReply.mutate(replyAll, { onSuccess: (draft) => { onOpenDraft(draft.messageId, true); } });
  };
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
    <div className="outlook-actions">
      <IconActions label="Message actions">
        <IconButton label="Conversation" icon={<ConversationIcon />} onClick={onThread} />
        {message.webUrl && <IconLink label="Open in Outlook" icon={<OutlookIcon />} href={message.webUrl} external />}
        <IconButton label={message.isRead ? "Mark unread" : "Mark read"} icon={message.isRead ? <MailIcon /> : <MailCheckIcon />}
          disabled={change.isPending} onClick={() => { change.mutate({ isRead: !message.isRead }); }} />
        {archive && archive.id !== message.folderId && <IconButton label="Archive" icon={<ArchiveIcon />} disabled={change.isPending}
          onClick={() => { change.mutate({ folderId: archive.id }); }} />}
      </IconActions>
    </div>
    <div className="row outlook-actions">
      <select aria-label="Destination folder" value={destination} onChange={(e) => { setDestination(e.target.value); }}>
        <option value="">Move to…</option>
        {folders.filter((folder) => folder.id !== message.folderId).map((folder) => <option key={folder.id} value={folder.id}>{folder.name}</option>)}
      </select>
      <IconButton label="Move" icon={<MoveFolderIcon />} disabled={!destination || change.isPending}
        onClick={() => { change.mutate({ folderId: destination }); }} />
      {profile && <>
        {linked
          ? <button type="button" disabled={link.isPending} onClick={() => { link.mutate(true); }}>Unlink from profile</button>
          : <button type="button" disabled={link.isPending} onClick={() => { link.mutate(false); }}>Link to {profile.candidateId ? "candidate" : "contact"}</button>}
      </>}
    </div>
    <ErrorMessage error={body.error ?? change.error ?? send.error ?? draftReply.error ?? link.error}/>
    {notice && <p role="status">{notice}</p>}
    {body.isPending && <p className="muted">Opening message…</p>}
    {draftReply.isPending && <p className="muted" role="status">Opening reply…</p>}
    {body.data && <p className="outlook-body">{body.data.text}</p>}
    {body.data?.truncated && <p className="muted">Open in Outlook to read the rest.</p>}
    {/* The candidate is suggested only for their own linked mail, never for any message read from their page. */}
    {message.hasAttachments && <OutlookAttachments key={message.id} messageId={message.id}
      canSaveCv={canSaveCv} canImportCv={canImportCv} candidateId={linked ? profile?.candidateId : undefined} />}
    {!message.isDraft && mode === null && <div className="outlook-response-actions">
      <IconActions label="Message responses">
        <IconButton label="Reply" icon={<ReplyIcon />} disabled={draftReply.isPending} onClick={() => { startReply(false); }} />
        <IconButton label="Reply all" icon={<ReplyAllIcon />} disabled={draftReply.isPending} onClick={() => { startReply(true); }} />
        {draftsReady && <IconButton label="Forward" icon={<ForwardIcon />} disabled={draftReply.isPending}
          onClick={() => { setMode("forward"); setNotice(""); }} />}
      </IconActions>
    </div>}
    {mode === "forward" && <OutlookForward messageId={message.id} onOpen={onOpenDraft} onClose={() => { setMode(null); }} />}
    {(mode === "reply" || mode === "replyAll") && <form className="outlook-reply" onSubmit={(e) => { e.preventDefault(); send.mutate(false); }}>
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
