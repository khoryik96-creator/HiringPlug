// Document files are fetched with the user's token (an <iframe> or a plain
// link cannot send it) and handed to the browser as blob URLs.
import { getToken } from "../auth";
import { type Document, type Schemas, throwFor } from "./client";

export async function uploadCandidateDocument(candidateId: string, source: Schemas["DocumentSource"], file: Blob, fileName: string, replace: string[] = []): Promise<void> {
  const body = new FormData();
  body.append("source", source);
  body.append("file", file, fileName);
  for (const id of replace) body.append("replaceDocumentIds", id);
  const res = await fetch(`/v1/candidates/${encodeURIComponent(candidateId)}/documents`, {
    method: "POST",
    body,
    headers: { Authorization: `Bearer ${getToken() ?? ""}` },
  });
  if (!res.ok) await throwFor(res);
}

/** Uploads a receipt as a new draft claim (it is read in the background). */
export async function uploadClaimReceipt(file: Blob, fileName: string): Promise<Schemas["Claim"]> {
  const body = new FormData();
  body.append("file", file, fileName);
  const res = await fetch("/v1/claims", { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  if (!res.ok) await throwFor(res);
  return await res.json() as Schemas["Claim"];
}

export async function fetchDocumentFile(doc: Pick<Document, "id" | "contentType">): Promise<Blob> {
  const res = await fetch(`/v1/documents/${doc.id}/file`, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  if (!res.ok) await throwFor(res); // throws the API's error
  // The server sends octet-stream; the stored type lets the browser show PDFs.
  return new Blob([await res.arrayBuffer()], { type: doc.contentType });
}

/** A Word document's PDF copy (for previewing it as it looks). */
export async function fetchDocumentPdf(id: string): Promise<Blob> {
  const res = await fetch(`/v1/documents/${id}/pdf`, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  if (!res.ok) await throwFor(res);
  return new Blob([await res.arrayBuffer()], { type: "application/pdf" });
}

/** A file the API serves, as a blob of the given type (to show it in the app). */
export async function fetchFile(path: string, type: string, signal?: AbortSignal): Promise<Blob> {
  const res = await fetch(path, { headers: { Authorization: `Bearer ${getToken() ?? ""}` }, signal });
  if (!res.ok) await throwFor(res);
  return new Blob([await res.arrayBuffer()], { type });
}

export function outlookAttachmentFilePath(messageId: string, attachmentId: string): string {
  return `/v1/outlook/messages/${encodeURIComponent(messageId)}/attachments/${encodeURIComponent(attachmentId)}/file`;
}

/** Every file attachment of a message in one .zip (Download all). */
export function outlookAttachmentsZipPath(messageId: string): string {
  return `/v1/outlook/messages/${encodeURIComponent(messageId)}/attachments/zip`;
}

export async function uploadOutlookDraftFile(messageId: string, version: string, file: File): Promise<Schemas["OutlookDraft"]> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(`/v1/outlook/drafts/${encodeURIComponent(messageId)}/attachments?version=${encodeURIComponent(version)}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${getToken() ?? ""}` },
    body,
  });
  if (!res.ok) await throwFor(res);
  return await res.json() as Schemas["OutlookDraft"];
}

/** Saves a file the API serves (e.g. an interview's calendar entry) under the given name. */
export async function downloadFrom(path: string, fileName: string, type: string): Promise<void> {
  const res = await fetch(path, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  if (!res.ok) await throwFor(res);
  const url = URL.createObjectURL(new Blob([await res.arrayBuffer()], { type }));
  const a = Object.assign(window.document.createElement("a"), { href: url, download: fileName });
  a.click();
  setTimeout(() => { URL.revokeObjectURL(url); }, 10_000);
}

/** Saves the document's file under its original name. */
export async function downloadDocument(doc: Pick<Document, "id" | "contentType" | "fileName">): Promise<void> {
  const url = URL.createObjectURL(await fetchDocumentFile(doc));
  const a = Object.assign(window.document.createElement("a"), { href: url, download: doc.fileName });
  a.click();
  setTimeout(() => { URL.revokeObjectURL(url); }, 10_000);
}

/**
 * Downloads an export (a GET with the list's filters, or a POST with a
 * search) and saves it under the name the API gives (Content-Disposition).
 */
export async function downloadExport(path: string, body?: unknown): Promise<void> {
  const headers: Record<string, string> = { Authorization: `Bearer ${getToken() ?? ""}` };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, { method: body === undefined ? "GET" : "POST", headers, body: body === undefined ? undefined : JSON.stringify(body) });
  if (!res.ok) await throwFor(res);
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const named = /filename\*=UTF-8''([^;]+)/i.exec(disposition)?.[1];
  const plain = /filename="?([^";]+)"?/i.exec(disposition)?.[1];
  const fileName = named ? decodeURIComponent(named) : plain ?? "export";
  const url = URL.createObjectURL(new Blob([await res.arrayBuffer()], { type: res.headers.get("Content-Type") ?? "application/octet-stream" }));
  const a = Object.assign(window.document.createElement("a"), { href: url, download: fileName });
  a.click();
  setTimeout(() => { URL.revokeObjectURL(url); }, 10_000);
}
