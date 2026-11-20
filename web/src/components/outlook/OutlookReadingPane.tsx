// The reading pane, as Outlook lays it out: the subject, then who it is from
// (picture, name and address), to and copied, and when, with Reply, Reply
// all and Forward as icons beside them; the attachments; then the message itself, in
// its own formatting.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatWhen } from "../../labels";
import { ConversationIcon, FlagIcon, ForwardIcon, IconButton, MoreIcon, OutlookIcon, ReplyAllIcon, ReplyIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { OutlookAttachments } from "./OutlookAttachments";
import { OutlookForward } from "./OutlookForward";
import { OutlookMessageFrame } from "./OutlookMessageFrame";
import type { OutlookProfile } from "./OutlookProfileMail";

type Message = Schemas["OutlookMessage"];
type Folder = Schemas["OutlookMailFolder"];
type Person = Schemas["OutlookPerson"];

/** "NA" for Natasha Leander's "Leander Natasha"; the address's first letters when there is no name. */
export function initials(name: string, address: string): string {
  const words = (name || address.split("@")[0] || "?").replace(/[^\p{L}\p{N} ._-]/gu, " ").split(/[\s._-]+/).filter(Boolean);
  return ((words[0]?.[0] ?? "?") + (words.length > 1 ? words[words.length - 1]?.[0] ?? "" : "")).toUpperCase();
}

const people = (list: Person[]) => list.map((p) => p.name || p.address).join("; ");

function hue(s: string): number {
  let h = 0;
  for (const c of s) h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}

export function OutlookReadingPane({ message, folders, profile, linked, canImportCv, userId, onThread, onOpenDraft }: {
  message: Message;
  folders: Folder[];
  profile?: OutlookProfile;
  linked: boolean;
  canImportCv: boolean;
  userId?: string;
  onThread: () => void;
  onOpenDraft: (id: string, focusMessage?: boolean, fresh?: boolean) => void;
}) {
  const client = useQueryClient();
  const [reply, setReply] = useState("");
  const [mode, setMode] = useState<"reply" | "replyAll" | "forward" | null>(null);
  const [notice, setNotice] = useState("");
  const draftsReady = folders.some((folder) => folder.kind === "drafts");
  const params = { path: { messageId: message.id } };
  const body = useQuery({
    queryKey: ["outlook", "body", message.id],
    queryFn: async () => unwrap(await api.GET("/v1/outlook/messages/{messageId}", { params })),
    gcTime: 0,
    retry: false,
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
  const forward = () => { setMode("forward"); setNotice(""); };
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

  const b = body.data;
  const from: Person = b?.from ?? { name: message.senderName, address: message.sender };
  const to = b?.to ?? [];
  const cc = b?.cc ?? [];
  const responding = mode !== null || draftReply.isPending;
  return <section className="outlook-reading" aria-label="Outlook message">
    <h3 className="outlook-reading-subject">
      {b?.importance === "high" && <span className="outlook-important" title="High importance" aria-label="High importance">!</span>}
      {message.subject || "(No subject)"}
      {message.flagged && <span className="outlook-flag-mark" title="Flagged" aria-label="Flagged"><FlagIcon /></span>}
    </h3>
    <div className="outlook-header">
      <span className="outlook-avatar" aria-hidden="true" style={{ background: `hsl(${String(hue(from.address || from.name))} 45% 42%)` }}>
        {initials(from.name, from.address)}
      </span>
      <div className="outlook-people">
        <div className="outlook-from-line">
          <strong>{from.name || from.address || "Draft"}</strong>
          {from.name && from.address && <span className="muted"> &lt;{from.address}&gt;</span>}
        </div>
        {to.length > 0 && <div className="outlook-to-line"><span className="muted">To: </span>{people(to)}</div>}
        {cc.length > 0 && <div className="outlook-to-line"><span className="muted">Cc: </span>{people(cc)}</div>}
        <div className="outlook-date muted">{formatWhen(b?.sentAt ?? message.receivedAt)}</div>
      </div>
      {!message.isDraft && (
        <div className="outlook-respond" role="toolbar" aria-label="Respond">
          <IconButton label="Reply" icon={<ReplyIcon />} disabled={responding} onClick={() => { startReply(false); }} />
          <IconButton label="Reply all" icon={<ReplyAllIcon />} disabled={responding} onClick={() => { startReply(true); }} />
          {draftsReady && <IconButton label="Forward" icon={<ForwardIcon />} disabled={responding} onClick={forward} />}
          <details className="outlook-more">
            <summary className="icon-action" aria-label="More actions" data-tip="More actions"><MoreIcon /></summary>
            <div className="outlook-move-menu" role="menu" aria-label="More actions">
              <button type="button" role="menuitem" onClick={onThread}><ConversationIcon /> Show the conversation</button>
              {message.webUrl && <a role="menuitem" href={message.webUrl} target="_blank" rel="noreferrer noopener"><OutlookIcon /> Open in Outlook</a>}
            </div>
          </details>
        </div>
      )}
    </div>
    {profile && <div className="row outlook-profile-link">
      {linked
        ? <button type="button" className="link" disabled={link.isPending} onClick={() => { link.mutate(true); }}>Unlink from profile</button>
        : <button type="button" className="link" disabled={link.isPending} onClick={() => { link.mutate(false); }}>Link to {profile.candidateId ? "candidate" : "contact"}</button>}
    </div>}
    <ErrorMessage error={body.error ?? send.error ?? draftReply.error ?? link.error}/>
    {notice && <p role="status">{notice}</p>}
    {draftReply.isPending && <p className="muted" role="status">Opening reply…</p>}
    {message.hasAttachments && <OutlookAttachments key={`files-${message.id}`} messageId={message.id} canImportCv={canImportCv} userId={userId} />}
    {body.isPending && <p className="muted">Opening message…</p>}
    {b?.html
      ? <OutlookMessageFrame key={`body-${message.id}`} html={b.html} blockedImages={b.blockedImages} title={`Message: ${message.subject || "(No subject)"}`}
        holdPictures={folders.find((f) => f.id === message.folderId)?.kind === "junkemail"} />
      : b && <div className="outlook-body">{b.text}</div>}
    {b?.truncated && <p className="muted">Open in Outlook to read the rest.</p>}
    {mode === "forward" && <OutlookForward messageId={message.id} onOpen={(id) => { onOpenDraft(id, false, true); }} onClose={() => { setMode(null); }} />}
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
