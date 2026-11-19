import { useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../../api/client";
import { fetchFile, outlookAttachmentFilePath } from "../../api/files";
import { getToken } from "../../auth";
import { CvImports } from "../CvImports";
import { isCvFileName } from "../DropZone";
import { ErrorMessage } from "../ErrorMessage";

export function OutlookCvImport({ messageId, attachmentId }: { messageId: string; attachmentId: string }) {
  const prepared = useQuery({
    queryKey: ["outlook", "cv-import", messageId, attachmentId],
    queryFn: async ({ signal }) => {
      const token = getToken();
      const { items } = unwrap(await api.GET("/v1/outlook/messages/{messageId}/attachments", {
        params: { path: { messageId } },
        signal,
      }));
      const attachment = items.find((item) => item.id === attachmentId);
      if (!attachment?.downloadable || !isCvFileName(attachment.fileName)) {
        throw new Error("This Outlook attachment is unavailable for CV import.");
      }
      if (getToken() !== token) throw new Error("Your sign-in changed. Open the message again to import this CV.");
      const blob = await fetchFile(outlookAttachmentFilePath(messageId, attachmentId), "application/octet-stream", signal);
      if (getToken() !== token) throw new Error("Your sign-in changed. Open the message again to import this CV.");
      return new File([blob], attachment.fileName, { type: blob.type });
    },
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  });

  if (prepared.data) {
    return <CvImports initialFiles={[prepared.data]} initialSource="" />;
  }

  return <section className="panel" aria-label="Import Outlook CV">
    {prepared.isFetching && <p className="muted" role="status">Opening Outlook attachment…</p>}
    <ErrorMessage error={prepared.error}>
      <button type="button" disabled={prepared.isFetching} onClick={() => { void prepared.refetch(); }}>Try again</button>
    </ErrorMessage>
    <a href="#/import">Import from your computer</a>
  </section>;
}
