// Reading a CV's details, as one button: "Read details" reads them, then
// "Review details" opens the review beside the CV. While the CV is read the
// button is orange with a timer and a bar fills, as for Format a CV; green
// once the details are ready. A reading that ended without details says why
// and offers reading again. One that runs far too long is checked less
// often while the server still has a job for it (waiting for the reader,
// running, or held after a restore: it says which), and offers reading
// again only once none is left.
import type { ReactNode } from "react";

import type { Document } from "../api/client";
import { IconButton, ReferenceIcon, TextIcon } from "./ActionIcons";
import { fill, ProgressLine, useNow } from "./StudioProgress";

const TYPICAL = 15; // seconds reading a CV usually takes
const FRESH = 120_000; // a CV this new has been read since its upload
/** Seconds after which a reading is long: checked less often, and taken for stopped when the server does not say a job is left. */
export const STALLED_AFTER = 10 * 60;
const POLL = 2000;
/** How often a long reading the server still has a job for is checked. */
export const SLOW_POLL = 30_000;

export const isReading = (d: Document) => d.status === "queued" || d.parseStatus === "queued";

/** Read for far longer than a CV usually takes. */
const isLong = (d: Document, now: number) => isReading(d) && readSeconds(d, now) > STALLED_AFTER;

/**
 * The reading stopped: the server has no job left for it (readingJob null),
 * or, where it does not say, it ran far too long. Reading again is offered.
 */
export const isStalled = (d: Document, now: number) =>
  isReading(d) && (d.readingJob === null || (d.readingJob === undefined && isLong(d, now)));

/**
 * How often to check a candidate's CVs: every 2 seconds while one is being
 * read (or its PDF copy made), for STALLED_AFTER; then every 30 seconds
 * while the server still has a job for it; never once it stopped.
 */
export function pollEvery(docs: Document[] | undefined, now = Date.now()): number | false {
  const list = docs ?? [];
  const reading = list.filter((d) => isReading(d) && !isStalled(d, now));
  if (reading.some((d) => !isLong(d, now))
    || list.some((d) => d.pdfStatus === "queued" && now - Date.parse(d.updatedAt) < STALLED_AFTER * 1000)) return POLL;
  return reading.length > 0 ? SLOW_POLL : false;
}

/** What a reading's job is doing, when the server says so and it is worth saying. */
function waitingFor(d: Document): string | null {
  if (d.readingJob === "held") return "Held after a restore of the server (Settings › System releases it)";
  if (d.readingJob === "waiting") return "Waiting for the reader";
  return null;
}

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

export function DetailsButton({ doc, ready, reviewing, canReview, canRead, busy, onRead, onReview }: {
  doc: Document;
  /** The details were read while this page was open: green until opened. */
  ready: boolean;
  reviewing: boolean;
  canReview: boolean;
  canRead: boolean;
  busy: boolean;
  onRead: () => void;
  onReview: () => void;
}) {
  if (doc.source === "formatted") return null; // our own output: read only for comparing (the ⋯ menu)
  if (isReading(doc)) return <ReadingButton doc={doc} again={canRead ? onRead : undefined} busy={busy} />;
  if (doc.parseStatus === "parsed" && canReview) {
    return (
      <IconButton label={reviewing ? "Close review" : "Review details"} icon={<ReferenceIcon />}
        className={ready ? "icon-done" : "icon-soft"} pressed={reviewing} onClick={onReview} />
    );
  }
  if (canRead && (doc.status === "extracted" || doc.status === "failed")) {
    return (
      <IconButton label={doc.status === "failed" ? "Read the CV again" : doc.parseStatus === "failed" ? "Read details again" : "Read details"}
        icon={<TextIcon />} className="icon-soft" disabled={busy} onClick={onRead} />
    );
  }
  return null;
}

/**
 * The orange button while the CV is read, its seconds ticking (only it
 * re-renders each second), or what its job waits for; once the reading
 * stopped, reading again instead (the server never queues a second reading).
 */
function ReadingButton({ doc, again, busy }: { doc: Document; again?: () => void; busy: boolean }) {
  const now = useNow(true);
  if (isStalled(doc, now)) {
    return again
      ? <IconButton label="Read details again" icon={<TextIcon />} className="icon-soft" disabled={busy} onClick={again} />
      : <span className="muted small" role="status">Reading stopped</span>;
  }
  const waiting = waitingFor(doc);
  if (waiting) {
    const short = doc.readingJob === "held" ? "Held after a restore" : "Waiting for the reader";
    return <button type="button" className="doc-btn format-running" disabled title={waiting}>{short}…</button>;
  }
  return <button type="button" className="doc-btn format-running" disabled>Reading details… {readSeconds(doc, now)}s</button>;
}

/** Why a CV not being read has no details, or why reading it again failed (the details before are kept). */
function ReadNote({ doc }: { doc: Document }) {
  const note = doc.status === "failed" ? doc.errorMessage ?? "The text could not be read."
    : doc.parseStatus === "parsed" || doc.parseStatus === "failed" ? doc.parseError : null;
  return note ? <p className="warning small" role="status">{note}</p> : null;
}

/** The bar under the buttons: filling while the CV is read, full (green) once its details are ready. */
export function ReadProgress({ doc, finished, action, onDismiss }: {
  doc: Document;
  finished: boolean;
  action?: ReactNode;
  onDismiss: () => void;
}) {
  const now = useNow(isReading(doc));
  const stalled = isStalled(doc, now);
  const reading = isReading(doc) && !stalled;
  if (!isReading(doc) && !finished) return <ReadNote doc={doc} />;
  const seconds = readSeconds(doc, now);
  const failed = stalled || (!reading && doc.parseStatus !== "parsed");
  const waiting = reading ? waitingFor(doc) ?? (isLong(doc, now) ? "Still reading: this one takes longer than usual" : null) : null;
  const why = stalled ? "The reader stopped: read it again."
    : waiting ? `${waiting}.`
      : failed ? doc.status === "failed" ? doc.errorMessage : doc.parseError ?? doc.errorMessage
        : doc.parseError; // read again and it failed: the details before are kept
  return (
    <ProgressLine state={reading ? "running" : failed ? "failed" : "done"} ariaLabel="Reading the CV" seconds={seconds}
      pct={reading ? fill(seconds, TYPICAL) : 100}
      label={reading ? (doc.status === "queued" ? "Reading the CV…" : "Reading the details…")
        : stalled ? "Reading stopped"
          : doc.status === "failed" ? "The CV could not be read"
            : failed ? "The details could not be read" : <>Details read from <strong>{doc.fileName}</strong></>}
      end={<>{action}<button type="button" className="link" onClick={onDismiss}>Dismiss</button></>}>
      {(failed || why) && <small className="warning" role={failed ? "alert" : "status"}>{why ?? "Read it again, or fill in the details by hand."}</small>}
    </ProgressLine>
  );
}
