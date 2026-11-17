import { useEffect, useState } from "react";

/**
 * Copies text: the Clipboard API where the browser offers it (https and
 * localhost), else the older copy command (the app opened over plain http).
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    // Absent outside https and localhost, whatever the types say.
    const clipboard = (navigator as { clipboard?: Clipboard }).clipboard;
    if (clipboard) {
      await clipboard.writeText(text);
      return true;
    }
  } catch { /* fall back below */ }
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.appendChild(area);
  area.select();
  try {
    // eslint-disable-next-line @typescript-eslint/no-deprecated -- the only copy without the Clipboard API (plain http)
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    area.remove();
  }
}

/** One click copies text (an email, a number); it says "Copied" for a moment. */
export function CopyButton({ text, what }: { text: string; what: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => { setCopied(false); }, 1500);
    return () => { clearTimeout(t); };
  }, [copied]);
  return (
    <button type="button" className={copied ? "copy-btn copied" : "copy-btn"} aria-label={copied ? `${what} copied` : `Copy ${what}`}
      title={copied ? "Copied" : `Copy ${what}`}
      onClick={() => { void copyText(text).then((ok) => { if (ok) setCopied(true); }); }}>
      {copied ? (
        <><svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg> Copied</>
      ) : (
        <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2.5" fill="none" stroke="currentColor" strokeWidth="2" /><path d="M5 15V6a2 2 0 0 1 2-2h9" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
      )}
    </button>
  );
}

/** The text itself as the button: clicking an email or a number copies it, and it says "Copied" for a moment. */
export function CopyText({ text, what }: { text: string; what: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => { setCopied(false); }, 1500);
    return () => { clearTimeout(t); };
  }, [copied]);
  return (
    <button type="button" className="copy-text" title={`Click to copy the ${what}`} aria-label={`${text}: copy ${what}`}
      onClick={() => { void copyText(text).then((ok) => { if (ok) setCopied(true); }); }}>
      <span className="copy-value">{text}</span>
      {copied && <span className="copy-done" role="status">Copied</span>}
    </button>
  );
}
