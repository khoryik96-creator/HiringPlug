// A file shown in the app (an invoice, a client's document) instead of
// downloaded: a dialog over the page with the PDF or picture, and buttons.
import { useEffect, useRef, useState, type ReactNode } from "react";

import { fetchFile } from "../api/files";
import { ErrorMessage } from "./ErrorMessage";

export function FileViewer({ title, path, type, actions, onClose }: {
  title: string; path: string; type: string; actions?: ReactNode; onClose: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let made: string | null = null;
    let live = true;
    fetchFile(path, type)
      .then((blob) => { if (live) { made = URL.createObjectURL(blob); setUrl(made); } })
      .catch((e: unknown) => { if (live) setError(e); });
    return () => { live = false; if (made) URL.revokeObjectURL(made); };
  }, [path, type]);
  useEffect(() => {
    const d = dialog.current;
    if (d && !d.open) {
      if (typeof d.showModal === "function") d.showModal();
      else d.setAttribute("open", "");
    }
  }, []);
  return (
    <dialog ref={dialog} className="file-viewer" aria-label={title} onClose={onClose} onCancel={onClose}>
      <header className="file-viewer-head">
        <strong>{title}</strong>
        <span className="row">
          {actions}
          <button type="button" onClick={onClose}>Close</button>
        </span>
      </header>
      <ErrorMessage error={error} />
      {!url && !error && <p className="muted file-viewer-wait">Making the preview…</p>}
      {url && (type.startsWith("image/")
        ? <img className="file-viewer-body" src={url} alt={title} />
        : <iframe className="file-viewer-body" title={title} src={`${url}#navpanes=0&view=FitH`} />)}
    </dialog>
  );
}
