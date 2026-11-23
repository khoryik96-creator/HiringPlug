// Create job on an Outlook attachment (the client's job description, as a
// PDF, Word or text file): the file is fetched and the Add job form opens on
// Jobs with it, read and filled in as if dropped there; the file is kept
// with the job once it is added.
import { useMutation } from "@tanstack/react-query";

import { fetchFile, outlookAttachmentFilePath } from "../../api/files";
import type { Schemas } from "../../api/client";
import { BriefcaseIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";
import { handJd } from "../jdHandover";

/** Files a job can be read from (as the Add job form takes them). */
export const isJdFileName = (name: string) => /\.(pdf|docx?|txt|rtf|odt)$/i.test(name);

export function CreateJobButton({ messageId, file }: { messageId: string; file: Schemas["OutlookAttachment"] }) {
  const open = useMutation({
    mutationFn: async () => {
      const blob = await fetchFile(outlookAttachmentFilePath(messageId, file.id), "application/octet-stream");
      handJd(new File([blob], file.fileName, { type: blob.type }));
      window.location.hash = "#/jobs";
    },
  });
  return <>
    <button type="button" className="outlook-import outlook-create-job" aria-label={`Create a job from ${file.fileName}`}
      title="Create a job from this job description" disabled={open.isPending} onClick={() => { open.mutate(); }}>
      <BriefcaseIcon /> {open.isPending ? "Opening…" : "Create job"}
    </button>
    <ErrorMessage error={open.error} />
  </>;
}
