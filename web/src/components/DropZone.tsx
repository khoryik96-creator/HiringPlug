// A box to drop files on (or click to choose them), for CVs and job descriptions.
import { useId, useState } from "react";

/** The files whose extension is one of `accept` (".pdf,.docx"), and the names of the others. */
export function acceptable(files: File[], accept: string): { ok: File[]; skipped: string[] } {
  const exts = accept.split(",").map((e) => e.trim().toLowerCase()).filter(Boolean);
  const ok: File[] = [];
  const skipped: string[] = [];
  for (const f of files) {
    if (exts.some((e) => f.name.toLowerCase().endsWith(e))) ok.push(f);
    else skipped.push(f.name);
  }
  return { ok, skipped };
}

export function DropZone({ accept, multiple = false, disabled = false, label, hint, onFiles }: {
  accept: string;
  multiple?: boolean;
  disabled?: boolean;
  /** What to drop, e.g. "Drop CVs here". */
  label: string;
  hint?: string;
  onFiles: (files: File[]) => void;
}) {
  const id = useId();
  const [over, setOver] = useState(false);
  const [skipped, setSkipped] = useState<string[]>([]);
  const [picker, setPicker] = useState(0); // a new input after each pick, so the same file can be picked again
  const take = (list: FileList | null) => {
    const { ok, skipped: bad } = acceptable([...(list ?? [])], accept);
    setSkipped(bad);
    const chosen = multiple ? ok : ok.slice(0, 1);
    if (chosen.length > 0) onFiles(chosen);
  };

  return (
    <div
      className={`drop-zone${over ? " over" : ""}${disabled ? " disabled" : ""}`}
      onDragOver={(e) => { e.preventDefault(); if (!disabled) setOver(true); }}
      onDragLeave={() => { setOver(false); }}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        if (!disabled) take(e.dataTransfer.files);
      }}
    >
      <label htmlFor={id}>
        <span className="drop-icon" aria-hidden="true">⇪</span>
        <strong>{label}</strong>
        <span className="muted">or <span className="drop-browse">choose {multiple ? "files" : "a file"}</span>{hint ? ` · ${hint}` : ""}</span>
      </label>
      <input key={picker} id={id} type="file" accept={accept} multiple={multiple} disabled={disabled} className="visually-hidden"
        aria-label={label} onChange={(e) => { take(e.target.files); setPicker((n) => n + 1); }} />
      {skipped.length > 0 && <p className="warning small">Skipped (not {accept.replaceAll(",", " / ")}): {skipped.join(", ")}</p>}
    </div>
  );
}
