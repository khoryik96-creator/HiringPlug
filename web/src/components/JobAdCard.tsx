// One job ad on the job page: a card with its title, status and facts that
// opens to the ad as written and its blind version (both, or one at a time).
import { useState } from "react";

import type { Schemas } from "../api/client";
import { careersUrl } from "../careersRoute";
import { CHANNEL, countryName } from "../labels";
import { AdVersions, type Shown } from "./AdVersions";
import { useFieldSettings } from "./useFieldSettings";

type Ad = Schemas["JobAd"];
const STATUS: Record<Ad["status"], string> = { draft: "Draft", published: "Published", closed: "Closed" };
const VIEWS: [Shown, string][] = [["both", "Side by side"], ["original", "As written"], ["blind", "Blind"]];

export function AdCard({ ad, startOpen, canEdit, busy, careersSlug, onEdit, onStatus }: {
  ad: Ad;
  startOpen: boolean;
  /** May change it now (not while another ad is being written). */
  canEdit: boolean;
  busy: boolean;
  careersSlug?: string;
  onEdit: () => void;
  onStatus: (status: Ad["status"]) => void;
}) {
  const [open, setOpen] = useState(startOpen);
  const [view, setView] = useState<Shown>("both");
  const fs = useFieldSettings();
  const facts = [
    [ad.location, ad.country ? countryName(ad.country) : null].filter(Boolean).join(", "),
    ad.workArrangement && fs.label("work_arrangement", ad.workArrangement),
    ad.salaryText,
    ad.closesOn && `closes ${ad.closesOn}`,
  ].filter(Boolean).join(" · ");
  return (
    <article className={`ad-card${open ? " open" : ""}`} aria-label={ad.title}>
      <header className="ad-card-head">
        <button type="button" className="fold-toggle ad-card-title" aria-expanded={open} onClick={() => { setOpen(!open); }}>
          {ad.title}
        </button>
        <span className={`badge ${ad.status}`}>{STATUS[ad.status]}</span>
        {ad.status === "published" && <span className="muted small">{ad.publishBlind ? "blind" : "as written"} on the careers page</span>}
        <span className="ad-card-facts muted small">{facts}</span>
        {canEdit && (
          <span className="row ad-card-actions">
            <button type="button" className="link" onClick={() => { setOpen(true); onEdit(); }}>Edit</button>
            {ad.status !== "published" && (
              <button type="button" disabled={busy} onClick={() => { onStatus("published"); }}>Publish</button>
            )}
            {ad.status === "published" && (
              <button type="button" className="link" onClick={() => { onStatus("draft"); }}>Unpublish</button>
            )}
            {ad.status !== "closed" && (
              <button type="button" className="link" onClick={() => { onStatus("closed"); }}>Close</button>
            )}
          </span>
        )}
      </header>
      {open && (
        <div className="ad-card-body">
          <div className="segmented" role="group" aria-label="Show">
            {VIEWS.map(([v, label]) => (
              <button key={v} type="button" aria-pressed={view === v} className={view === v ? "on" : undefined}
                onClick={() => { setView(v); }}>{label}</button>
            ))}
          </div>
          <AdVersions ad={ad} canEdit={canEdit} show={view} />
          {ad.status === "published" && careersSlug && <ShareLinks slug={careersSlug} adId={ad.id} />}
        </div>
      )}
    </article>
  );
}

const BOARDS = ["jobstreet", "linkedin", "indeed", "facebook"] as const;

/**
 * Links to this ad for each job board: post the ad there with its link, and
 * applicants who come through it are recorded as coming from that board.
 */
function ShareLinks({ slug, adId }: { slug: string; adId: string }) {
  const [copied, setCopied] = useState<string | null>(null);
  return (
    <details className="share">
      <summary>Links for job boards</summary>
      <table>
        <tbody>
          {BOARDS.map((board) => {
            const url = careersUrl(window.location.origin, slug, adId, board);
            return (
              <tr key={board}>
                <td>{CHANNEL[board]}</td>
                <td><code>{url}</code></td>
                <td>
                  <button type="button" className="link" onClick={() => {
                    void navigator.clipboard.writeText(url).then(() => { setCopied(board); });
                  }}>{copied === board ? "Copied" : "Copy"}</button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </details>
  );
}
