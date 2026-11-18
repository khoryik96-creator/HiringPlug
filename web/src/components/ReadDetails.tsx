// Reading a CV's details, as one button: "Read details" reads them, then
// "Review details" opens the review beside the CV. While the CV is read the
// button is orange with a timer and a bar fills, as for Format a CV; green
// once the details are ready.
import type { ReactNode } from "react";

import type { Document } from "../api/client";

const TYPICAL = 15; // seconds reading a CV usually takes
const FRESH = 120_000; // a CV this new has been read since its upload

export const isReading = (d: Document) => d.status === "queued" || d.parseStatus === "queued";

/** When reading began: its upload for a new CV, else when the details were asked for. */
function startOf(d: Document) {
  const created = Date.parse(d.createdAt);
  const updated = Date.parse(d.updatedAt);
  return d.status === "queued" || updated - created < FRESH ? created : updated;
}

/** Seconds this CV has been read for, at now (0 when it is not being read). */
export function readSeconds(d: Document, now: number) {
  return isReading(d) ? Math.max(0, Math.round((now - startOf(d)) / 1000)) : 0;
}

export function DetailsButton({ doc, seconds, ready, reviewing, canReview, canRead, busy, onRead, onReview }: {
  doc: Document;
  seconds: number;
  /** The details were read while this page was open: green until opened. */
  ready: boolean;
  reviewing: boolean;
  canReview: boolean;
  canRead: boolean;
  busy: boolean;
  onRead: () => void;
  onReview: () => void;
}) {
  if (doc.source === "formatted") return null;
  if (isReading(doc)) return <button type="button" className="doc-btn format-running" disabled>Reading details… {seconds}s</button>;
  if (doc.parseStatus === "parsed" && canReview) {
    return (
      <button type="button" className={`doc-btn ${ready ? "format-done" : "primary-soft"}`} aria-pressed={reviewing} onClick={onReview}>
        {reviewing ? "Close review" : "Review details"}
      </button>
    );
  }
  if (canRead && doc.status === "extracted") {
    return (
      <button type="button" className="doc-btn primary-soft" disabled={busy} onClick={onRead}>
        {doc.parseStatus === "failed" ? "Read details again" : "Read details"}
      </button>
    );
  }
  return null;
}

/** The bar under the buttons: filling while the CV is read, full (green) once its details are ready. */
export function ReadProgress({ doc, seconds, finished, action, onDismiss }: {
  doc: Document;
  seconds: number;
  finished: boolean;
  action?: ReactNode;
  onDismiss: () => void;
}) {
  const reading = isReading(doc);
  if (!reading && !finished) return null;
  const failed = !reading && doc.parseStatus !== "parsed";
  const state = reading ? "running" : failed ? "failed" : "done";
  const pct = reading ? Math.min(95, 95 * (1 - Math.exp(-seconds / (TYPICAL / 2)))) : 100;
  return (
    <div className={`studio-progress ${state}`} role="status" aria-label="Reading the CV">
      <div className="studio-line">
        <span className="studio-label">
          {reading ? (doc.status === "queued" ? "Reading the CV…" : "Reading the details…")
            : failed ? "The details could not be read" : <>Details read from <strong>{doc.fileName}</strong></>}
        </span>
        <span className="studio-bar" aria-hidden="true"><i style={{ width: `${String(pct)}%` }} /></span>
        {reading ? <span className="studio-timer" aria-label={`${String(seconds)} seconds`}>{seconds}s</span>
          : <>{action}<button type="button" className="link" onClick={onDismiss}>Dismiss</button></>}
      </div>
      {failed && <small className="warning" role="alert">{doc.parseError ?? doc.errorMessage ?? "Read it again, or fill in the details by hand."}</small>}
    </div>
  );
}
