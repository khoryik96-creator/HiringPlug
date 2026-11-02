// Document files are fetched with the user's token (an <iframe> or a plain
// link cannot send it) and handed to the browser as blob URLs.
import { getToken } from "../auth";
import { type Document, unwrap } from "./client";

export async function fetchDocumentFile(doc: Pick<Document, "id" | "contentType">): Promise<Blob> {
  const res = await fetch(`/v1/documents/${doc.id}/file`, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
  if (!res.ok) unwrap({ error: await res.json(), response: res }); // throws the API's error
  // The server sends octet-stream; the stored type lets the browser show PDFs.
  return new Blob([await res.arrayBuffer()], { type: doc.contentType });
}

/** Saves the document's file under its original name. */
export async function downloadDocument(doc: Pick<Document, "id" | "contentType" | "fileName">): Promise<void> {
  const url = URL.createObjectURL(await fetchDocumentFile(doc));
  const a = Object.assign(window.document.createElement("a"), { href: url, download: doc.fileName });
  a.click();
  setTimeout(() => { URL.revokeObjectURL(url); }, 10_000);
}
