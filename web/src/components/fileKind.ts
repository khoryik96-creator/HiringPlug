// The short badge shown for a file's type (PDF, DOCX, IMG…), from its content type.
const KIND: Record<string, string> = {
  "application/pdf": "PDF", "application/msword": "DOC", "application/rtf": "RTF", "text/plain": "TXT",
  "application/vnd.oasis.opendocument.text": "ODT",
};

export const fileKind = (contentType: string) => KIND[contentType] ?? (contentType.startsWith("image/") ? "IMG" : "DOCX");
