// Files dropped anywhere on a page (the candidates page takes CVs this way):
// a full-page outline while files are dragged over. Rows dragged onto
// folders are not files, so they are left alone.
import { type DragEvent, useRef, useState } from "react";

import { acceptable } from "./DropZone";

const carriesFiles = (e: DragEvent) => e.dataTransfer.types.includes("Files");

export function usePageDrop(accept: string, onFiles: (files: File[]) => void, enabled: boolean) {
  const [over, setOver] = useState(false);
  const depth = useRef(0); // enter/leave fire for every child crossed
  const [skipped, setSkipped] = useState<string[]>([]);
  const take = (files: File[]) => {
    const { ok, skipped: bad } = acceptable(files, accept);
    setSkipped(bad);
    if (ok.length > 0) onFiles(ok);
  };
  if (!enabled) return { props: {}, overlay: null, skipped, take };
  return {
    take,
    skipped,
    props: {
      onDragEnter: (e: DragEvent) => { if (carriesFiles(e)) { depth.current += 1; setOver(true); } },
      onDragOver: (e: DragEvent) => { if (carriesFiles(e)) { e.preventDefault(); e.dataTransfer.dropEffect = "copy"; } },
      onDragLeave: (e: DragEvent) => { if (carriesFiles(e)) { depth.current = Math.max(0, depth.current - 1); if (depth.current === 0) setOver(false); } },
      onDrop: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        e.preventDefault();
        depth.current = 0;
        setOver(false);
        take([...e.dataTransfer.files]);
      },
    },
    overlay: over ? <div className="page-drop" aria-hidden="true"><span>Drop the CVs to add candidates</span></div> : null,
  };
}

/** "Drop CVs anywhere to upload", and any files that could not be taken. */
export function DropHint({ label, skipped }: { label: string; skipped: string[] }) {
  return (
    <span className="list-top-end drop-hint">
      <span aria-hidden="true">⬆</span> {label}
      {skipped.length > 0 && <span className="warning small" role="alert"> Not a CV file: {skipped.join(", ")}</span>}
    </span>
  );
}
