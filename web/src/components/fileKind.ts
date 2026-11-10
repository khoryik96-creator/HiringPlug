// The short badge shown for a file's type (PDF, DOCX, IMG…), from its content type.
const KIND: Record<string, string> = {
  "application/pdf": "PDF", "application/msword": "DOC", "application/rtf": "RTF", "text/plain": "TXT",
  "application/vnd.oasis.opendocument.text": "ODT",
};

export const fileKind = (contentType: string) => KIND[contentType] ?? (contentType.startsWith("image/") ? "IMG" : "DOCX");

/** A file's size for people: "84 KB", "1.2 MB". */
export const fileSize = (bytes: number) =>
  bytes < 1024 * 1024 ? `${String(Math.max(1, Math.round(bytes / 1024)))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`;
