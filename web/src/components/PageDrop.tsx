// Files dropped anywhere on a page (the candidates page takes CVs this way):
// a full-page outline while files are dragged over. Rows dragged onto
// folders are not files, so they are left alone.
import { type DragEvent, useId, useRef, useState } from "react";

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
        if (e.defaultPrevented) { depth.current = 0; setOver(false); return; } // a drop box on the page took them
        e.preventDefault();
        depth.current = 0;
        setOver(false);
        take([...e.dataTransfer.files]);
      },
    },
    overlay: over ? <div className="page-drop" aria-hidden="true"><span>Drop the CVs to add candidates</span></div> : null,
  };
}

/** "Drop CVs anywhere to upload" (clicking it picks files, for keyboards and touch screens), and any files that could not be taken. */
export function DropHint({ accept, label, onPick, skipped }: { accept: string; label: string; onPick: (files: File[]) => void; skipped: string[] }) {
  const id = useId();
  const [picker, setPicker] = useState(0); // a new input after each pick, so the same file can be picked again
  return (
    <span className="list-top-end drop-hint">
      <label htmlFor={id} className="drop-hint-label" title="Or click to choose the files"><span aria-hidden="true">⬆</span> {label}</label>
      <input key={picker} id={id} type="file" accept={accept} multiple className="visually-hidden"
        onChange={(e) => { onPick([...(e.target.files ?? [])]); setPicker((n) => n + 1); }} />
      {skipped.length > 0 && <span className="warning small" role="alert"> Not a CV file: {skipped.join(", ")}</span>}
    </span>
  );
}
