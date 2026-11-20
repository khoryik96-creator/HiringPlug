// Import CV on an Outlook attachment: one click imports it as a candidate.
// The button shows how it goes (orange while importing, a green outline and
// "Uploaded" when done, a red outline and "Failed"); clicking it then shows
// the candidate to open, or why it failed and Try again.
import { useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";

import type { Schemas } from "../../api/client";
import { AlertIcon, CheckIcon, PersonPlusIcon } from "../ActionIcons";
import { useFieldSettings } from "../useFieldSettings";
import { type CvImportView, emailSource, importOutlookCv, stateOf, useCvImports } from "./outlookCvImports";

/** What became of an import, in a line: the candidate to open, or why it failed. */
export function CvImportOutcome({ view }: { view: CvImportView }) {
  const who = view.candidateId
    ? <a href={`#/candidates/${encodeURIComponent(view.candidateId)}`}>{view.candidateName ?? "Open the candidate"}</a>
    : null;
  switch (view.status) {
    case "uploading": return <span>Sending {view.fileName}…</span>;
    case "reading": return <span>Reading the CV…{who && <> {who}</>}</span>;
    case "created": return <span>New candidate: {who}</span>;
    case "unread": return <span>New candidate (fill in the details): {who}</span>;
    case "added": return <span>Added to {who}</span>;
    case "duplicate": return <span>Already in HiringPlug: {who}</span>;
    case "failed":
    case "error": return <span className="error">{view.error ?? "The CV could not be imported."}</span>;
  }
}

const CAPTION = { importing: "Importing…", done: "Uploaded", failed: "Failed" } as const;

export function ImportCvButton({ userId, messageId, file }: { userId: string; messageId: string; file: Schemas["OutlookAttachment"] }) {
  const client = useQueryClient();
  const fs = useFieldSettings();
  const view = useCvImports(userId).find((v) => v.messageId === messageId && v.attachmentId === file.id);
  const panel = useRef<HTMLDetailsElement>(null);
  const start = () => {
    panel.current?.removeAttribute("open");
    void importOutlookCv(client, { userId, messageId, attachmentId: file.id, fileName: file.fileName, source: emailSource(fs.options("candidate_source", null)) });
  };
  if (!view) {
    return <button type="button" className="outlook-import" aria-label={`Import ${file.fileName} as a candidate`} onClick={start}>
      <PersonPlusIcon /> Import CV
    </button>;
  }
  const state = stateOf(view.status);
  return (
    <details ref={panel} className="outlook-import-wrap">
      <summary className={`outlook-import ${state}`} aria-label={`${CAPTION[state]}: ${file.fileName}`} aria-busy={state === "importing"}>
        {state === "importing" ? <span className="outlook-import-spinner" aria-hidden="true" /> : state === "done" ? <CheckIcon /> : <AlertIcon />}
        {CAPTION[state]}
      </summary>
      <div className="outlook-import-panel" role="status">
        <CvImportOutcome view={view} />
        {state === "failed" && <button type="button" onClick={start}>Try again</button>}
      </div>
    </details>
  );
}
