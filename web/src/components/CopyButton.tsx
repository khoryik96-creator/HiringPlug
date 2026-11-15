import { useEffect, useState } from "react";

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
      onClick={() => { void navigator.clipboard.writeText(text).then(() => { setCopied(true); }, () => undefined); }}>
      {copied ? (
        <><svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg> Copied</>
      ) : (
        <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2.5" fill="none" stroke="currentColor" strokeWidth="2" /><path d="M5 15V6a2 2 0 0 1 2-2h9" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
      )}
    </button>
  );
}
