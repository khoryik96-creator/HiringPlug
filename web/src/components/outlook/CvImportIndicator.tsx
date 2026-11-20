// Beside Settings in the top bar: the CVs imported from Outlook, at a glance.
// An outline shows how they are going (orange: importing, red: one failed,
// green: imported), with the number imported and failed since last looked
// at; the browser tab's title shows the same. Opening it lists them, with
// the candidate each became.
import { useEffect, useRef } from "react";

import { CvImportOutcome } from "./ImportCvButton";
import { markCvImportsSeen, stateOf, useCvImports } from "./outlookCvImports";

/** The tab's title with the counts in front: "(1… 2✓ 1✗) HiringPlug". */
export function titleWithCounts(title: string, counts: { importing: number; done: number; failed: number }): string {
  const bare = title.replace(/^\([^)]*[…✓✗][^)]*\) /, "");
  const parts = [counts.importing && `${String(counts.importing)}…`, counts.done && `${String(counts.done)}✓`, counts.failed && `${String(counts.failed)}✗`].filter(Boolean);
  return parts.length ? `(${parts.join(" ")}) ${bare}` : bare;
}

export function CvImportIndicator({ userId }: { userId: string }) {
  const views = useCvImports(userId);
  const menu = useRef<HTMLDetailsElement>(null);
  const importing = views.filter((v) => stateOf(v.status) === "importing").length;
  const done = views.filter((v) => !v.seen && stateOf(v.status) === "done").length;
  const failed = views.filter((v) => !v.seen && stateOf(v.status) === "failed").length;
  useEffect(() => { document.title = titleWithCounts(document.title, { importing, done, failed }); }, [importing, done, failed]);
  if (views.length === 0) return null;
  const state = importing ? "importing" : failed ? "failed" : done ? "done" : "idle";
  const label = [importing && `${String(importing)} importing`, done && `${String(done)} imported`, failed && `${String(failed)} failed`].filter(Boolean).join(", ");
  return (
    <details ref={menu} className="cv-indicator" onToggle={(e) => { if (!e.currentTarget.open) markCvImportsSeen(userId); }}>
      <summary className={`settings-link topbar-shortcut cv-indicator-button ${state}`} aria-label={`CV imports from Outlook${label ? `: ${label}` : ""}`}
        title={label ? `CV imports: ${label}` : "CV imports"}>
        <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
          <path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9ZM14 3v6h6" /><circle cx="12" cy="12.5" r="2" /><path d="M8.5 18a3.5 3.5 0 0 1 7 0" />
        </svg>
        {done > 0 && <span className="cv-count done" aria-hidden="true">{done}</span>}
        {failed > 0 && <span className="cv-count failed" aria-hidden="true">{failed}</span>}
        {importing > 0 && done + failed === 0 && <span className="cv-count importing" aria-hidden="true">{importing}</span>}
      </summary>
      <div className="cv-indicator-menu" role="dialog" aria-label="CV imports from Outlook">
        <strong>CV imports from Outlook</strong>
        <ul>
          {[...views].reverse().slice(0, 12).map((v) => (
            <li key={`${v.messageId}/${v.attachmentId}`} className={`cv-${stateOf(v.status)}`}>
              <span className="cv-file">{v.fileName}</span>
              <CvImportOutcome view={v} />
            </li>
          ))}
        </ul>
        <a href="#/import">All imports</a>
      </div>
    </details>
  );
}
