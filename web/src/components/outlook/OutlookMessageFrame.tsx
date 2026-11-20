// A received message as it was written: its own fonts, colours, tables and
// pictures, in a frame of its own so its styles stay inside it. The server
// has already removed anything that runs; the frame allows no scripts
// either. Pictures from the web load as in Outlook, except in Junk Email,
// where they wait until the person asks (a sender cannot tell it was opened).
import { useEffect, useMemo, useRef, useState } from "react";

const FRAME_STYLE = `
  html { color-scheme: light; }
  body { margin: 0; padding: 2px; font: 14px/1.5 "Segoe UI", "Inter Variable", system-ui, -apple-system, sans-serif;
    color: #242424; background: #fff; overflow-wrap: anywhere; }
  img { max-width: 100%; height: auto; }
  img[data-hp-src] { display: inline-block; min-width: 24px; min-height: 16px; background: #f1eef8; border: 1px dashed #d0c7e8; }
  table { max-width: 100%; }
  pre { white-space: pre-wrap; }
  a { color: #5d3dc2; }
  blockquote { margin-left: 0; padding-left: 12px; border-left: 3px solid #d0c7e8; color: #444; }
`;

/** The message's HTML with the held-back web pictures shown (or not). */
export function frameDocument(html: string, showPictures: boolean): string {
  let body = html;
  if (showPictures) {
    const doc = new DOMParser().parseFromString(`<body>${html}</body>`, "text/html");
    doc.querySelectorAll<HTMLImageElement>("img[data-hp-src]").forEach((img) => {
      const src = img.getAttribute("data-hp-src") ?? "";
      if (src.startsWith("https://")) img.setAttribute("src", src);
      img.removeAttribute("data-hp-src");
    });
    body = doc.body.innerHTML;
  }
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"><base target="_blank"><style>${FRAME_STYLE}</style></head><body>${body}</body></html>`;
}

export function OutlookMessageFrame({ html, blockedImages, title, holdPictures = false }: {
  html: string; blockedImages: number; title: string; holdPictures?: boolean;
}) {
  const [showPictures, setShowPictures] = useState(!holdPictures);
  const frame = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(160);
  const doc = useMemo(() => frameDocument(html, showPictures), [html, showPictures]);

  // The frame is as tall as the message, so the reading pane scrolls as one.
  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    let observer: ResizeObserver | undefined;
    const fit = () => {
      const body = el.contentDocument?.body;
      if (!body) return;
      setHeight(Math.max(60, Math.ceil(body.scrollHeight) + 8));
      if (!observer && typeof ResizeObserver !== "undefined") {
        observer = new ResizeObserver(() => { setHeight(Math.max(60, Math.ceil(body.scrollHeight) + 8)); });
        observer.observe(body);
      }
    };
    el.addEventListener("load", fit);
    fit();
    return () => { el.removeEventListener("load", fit); observer?.disconnect(); };
  }, [doc]);

  return (
    <>
      {blockedImages > 0 && !showPictures && (
        <p className="outlook-blocked" role="note">
          <span>This message is in Junk Email: its pictures from the web were not loaded.</span>
          <button type="button" className="link" onClick={() => { setShowPictures(true); }}>Show pictures</button>
        </p>
      )}
      {/* allow-same-origin without allow-scripts: nothing in it runs; the page only measures its height. */}
      <iframe ref={frame} className="outlook-frame" title={title} srcDoc={doc} style={{ height }}
        sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox" referrerPolicy="no-referrer" />
    </>
  );
}
