// CVs imported from Outlook attachments, followed from the click to the
// candidate: in progress (orange), done (green) or failed (red). What was
// imported from which message is kept in this browser (ids only, per
// person), so the message, the button and the indicator in the top bar show
// it again later; the outcome and the candidate come from the import itself.
import { type QueryClient, useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";

import { api, type Schemas, throwFor, unwrap } from "../../api/client";
import { fetchFile, outlookAttachmentFilePath } from "../../api/files";
import { cvImportsKey } from "../../api/keys";
import { getToken } from "../../auth";

type ItemStatus = Schemas["CvImportItemStatus"];

/** One attachment imported (or being imported), as kept in this browser. */
export type CvImportEntry = {
  userId: string;
  messageId: string;
  attachmentId: string;
  fileName: string;
  /** uploading: the file is on its way; then the import's own status; error: it could not be sent. */
  status: "uploading" | ItemStatus | "error";
  importId?: string;
  itemId?: string;
  candidateId?: string;
  error?: string;
  at: string;
  /** Looked at in the indicator: no longer counted there. */
  seen: boolean;
};

/** An entry with what the import says now (the candidate's name is never kept in the browser). */
export type CvImportView = CvImportEntry & { candidateName?: string };

export type CvImportState = "importing" | "done" | "failed";

export function stateOf(status: CvImportEntry["status"]): CvImportState {
  if (status === "uploading" || status === "reading") return "importing";
  return status === "failed" || status === "error" ? "failed" : "done";
}

/** What the message's imports add up to: in progress first, then a failure, then done. */
export function messageState(views: CvImportView[], messageId: string): CvImportState | undefined {
  const states = views.filter((v) => v.messageId === messageId).map((v) => stateOf(v.status));
  if (states.length === 0) return undefined;
  if (states.includes("importing")) return "importing";
  return states.includes("failed") ? "failed" : "done";
}

const KEY = "hiringplug.outlook.cvImports";
const KEEP = 100;
const listeners = new Set<() => void>();
let entries: CvImportEntry[] = load();

function load(): CvImportEntry[] {
  try {
    const raw = window.localStorage.getItem(KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? (parsed as CvImportEntry[]) : [];
  } catch {
    return [];
  }
}

function save(next: CvImportEntry[]) {
  entries = next.slice(-KEEP);
  try { window.localStorage.setItem(KEY, JSON.stringify(entries)); } catch { /* only a convenience */ }
  for (const l of listeners) l();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  const other = (e: StorageEvent) => { if (e.key === KEY) { entries = load(); listener(); } }; // another tab
  window.addEventListener("storage", other);
  return () => { listeners.delete(listener); window.removeEventListener("storage", other); };
}

const same = (e: CvImportEntry, userId: string, messageId: string, attachmentId: string) =>
  e.userId === userId && e.messageId === messageId && e.attachmentId === attachmentId;

function update(userId: string, messageId: string, attachmentId: string, change: Partial<CvImportEntry>) {
  save(entries.map((e) => (same(e, userId, messageId, attachmentId) ? { ...e, ...change } : e)));
}

/** For tests: forget everything. */
export function resetCvImports() { save([]); }

/** The candidate source a CV from an email gets: Direct, if the company has it. */
export function emailSource(options: { key: string }[]): string {
  return options.find((o) => o.key === "direct")?.key ?? options.find((o) => o.key === "other")?.key ?? options[0]?.key ?? "direct";
}

/**
 * Imports an Outlook attachment as a candidate: fetched from Outlook, then
 * sent to a new import. Runs on when the message is closed; the outcome is
 * followed by useCvImports.
 */
export async function importOutlookCv(client: QueryClient, { userId, messageId, attachmentId, fileName, source }: {
  userId: string; messageId: string; attachmentId: string; fileName: string; source: string;
}): Promise<void> {
  const entry: CvImportEntry = { userId, messageId, attachmentId, fileName, status: "uploading", at: new Date().toISOString(), seen: false };
  save([...entries.filter((e) => !same(e, userId, messageId, attachmentId)), entry]);
  const token = getToken();
  try {
    const blob = await fetchFile(outlookAttachmentFilePath(messageId, attachmentId), "application/octet-stream");
    if (getToken() !== token) throw new Error("Your sign-in changed. Open the message again to import this CV.");
    const imp = unwrap(await api.POST("/v1/cv-imports", { body: { source } }));
    const body = new FormData();
    body.append("file", new File([blob], fileName, { type: blob.type }));
    const res = await fetch(`/v1/cv-imports/${encodeURIComponent(imp.id)}/files`, { method: "POST", body, headers: { Authorization: `Bearer ${token ?? ""}` } });
    if (!res.ok) await throwFor(res);
    const item = (await res.json()) as Schemas["CvImportItem"];
    update(userId, messageId, attachmentId, {
      importId: imp.id, itemId: item.id, status: item.status, candidateId: item.candidateId ?? undefined, error: item.error ?? undefined,
    });
  } catch (e) {
    update(userId, messageId, attachmentId, { status: "error", error: e instanceof Error ? e.message : String(e) });
  }
  void client.invalidateQueries({ queryKey: outlookCvImportsKey });
  void client.invalidateQueries({ queryKey: cvImportsKey });
}

export const outlookCvImportsKey = ["outlook", "cv-imports"] as const;

/** Marks every import shown in the indicator as looked at. */
export function markCvImportsSeen(userId: string) {
  if (entries.some((e) => e.userId === userId && !e.seen && stateOf(e.status) !== "importing")) {
    save(entries.map((e) => (e.userId === userId && stateOf(e.status) !== "importing" ? { ...e, seen: true } : e)));
  }
}

/** The signed-in person's Outlook CV imports, newest last, with what each import says now. */
export function useCvImports(userId: string | undefined): CvImportView[] {
  const all = useSyncExternalStore(subscribe, () => entries);
  const mine = all.filter((e) => e.userId === userId);
  const following = mine.some((e) => e.importId && stateOf(e.status) === "importing");
  const live = useQuery({
    queryKey: outlookCvImportsKey,
    enabled: mine.some((e) => e.importId),
    refetchInterval: following ? 3000 : false,
    queryFn: async () => {
      const items = unwrap(await api.GET("/v1/cv-imports")).items.flatMap((i) => i.items);
      // Keeps the outcome, so it shows once the import is no longer among the latest.
      for (const e of entries) {
        const item = items.find((it) => it.id === e.itemId);
        if (item && (item.status !== e.status || (item.candidateId ?? undefined) !== e.candidateId)) {
          update(e.userId, e.messageId, e.attachmentId, { status: item.status, candidateId: item.candidateId ?? undefined, error: item.error ?? undefined });
        }
      }
      return items;
    },
  });
  return mine.map((e) => {
    const item = live.data?.find((it) => it.id === e.itemId);
    return item ? { ...e, candidateName: item.candidateName ?? undefined } : e;
  });
}
