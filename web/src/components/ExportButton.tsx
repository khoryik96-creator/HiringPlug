// "Export" on a list or report: what it shows, as an Excel workbook or a
// CSV. The API builds the file (with only what the user may see) and logs it.
import { useEffect, useRef, useState } from "react";

import { DownloadIcon } from "./ActionIcons";
import { ErrorMessage } from "./ErrorMessage";

export type ExportFormat = "xlsx" | "csv";

export function ExportButton({ run, what }: { run: (format: ExportFormat) => Promise<void>; what: string }) {
  const menu = useRef<HTMLDetailsElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (menu.current?.open && !menu.current.contains(e.target as Node)) menu.current.open = false;
    };
    document.addEventListener("click", close);
    return () => { document.removeEventListener("click", close); };
  }, []);
  const go = (format: ExportFormat) => {
    if (menu.current) menu.current.open = false;
    setBusy(true);
    setError(null);
    run(format).catch(setError).finally(() => { setBusy(false); });
  };
  return (
    <span className="export">
      <details ref={menu} className="export-menu">
        <summary className="search-pill" aria-label={`Export ${what}`} title={`Export ${what} to Excel or CSV`}>
          <DownloadIcon /> {busy ? "Exporting…" : "Export"}
        </summary>
        <div className="export-options" role="menu" aria-label="Export format">
          <button type="button" role="menuitem" disabled={busy} onClick={() => { go("xlsx"); }}>Excel (.xlsx)</button>
          <button type="button" role="menuitem" disabled={busy} onClick={() => { go("csv"); }}>CSV</button>
        </div>
      </details>
      {error !== null && <ErrorMessage error={error} />}
    </span>
  );
}
