// Moving a candidate to Offer or Placed asks for its details first: the
// offered salary and terms, or the placement report. Saving them moves the
// candidate (the server moves the application when an offer or placement is
// recorded). Leaving the offer form keeps them where they were; leaving the
// placement report puts them at Offer.
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";

import { api, type Application, type Permission, unwrap } from "../api/client";
import { applicationOffersKey } from "../api/keys";
import { STAGE } from "../labels";
import { OfferForm } from "./Offers";
import { PlacementForm } from "./PlacementForm";
import type { MoveRequest } from "./useMoveApplication";

/** Whether a move needs details first (and the user may give them). */
export function needsDetails(m: MoveRequest, permissions: Permission[]): boolean {
  // Only from an open stage: a placed candidate cannot get a new offer, and a
  // closed application is reopened by a plain move.
  if (m.app.stage === m.stage || ["placed", "rejected", "withdrawn"].includes(m.app.stage)) return false;
  if (m.stage === "offer") return permissions.includes("offers:manage");
  if (m.stage === "placed") return permissions.includes("placements:manage");
  return false;
}

export function StagePrompt({ app, stage, permissions, onMove, onClose }: {
  app: Application; stage: "offer" | "placed"; permissions: Permission[];
  /** A plain move (an offer already open, or back to Offer). */
  onMove: (m: MoveRequest) => void;
  onClose: () => void;
}) {
  const offers = useQuery({
    queryKey: applicationOffersKey(app.id),
    queryFn: async () => unwrap(await api.GET("/v1/offers", { params: { query: { applicationId: app.id } } })).items,
    enabled: stage === "offer",
  });
  const openOffer = offers.data?.some((o) => o.status !== "declined" && o.status !== "withdrawn");
  // An offer is already open: the move needs nothing new.
  useEffect(() => {
    if (stage === "offer" && openOffer) { onMove({ app, stage }); onClose(); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openOffer]);
  if (stage === "offer") {
    if (!offers.data || openOffer) return <p className="muted small" role="status">Checking {app.candidateName}'s offers…</p>;
    return (
      <section className="stage-prompt" aria-label={`Offer for ${app.candidateName}`}>
        <h4>Moving {app.candidateName} to Offer: the offered salary and terms</h4>
        <p className="muted small">Saving the offer moves them to Offer. Cancel leaves them in {STAGE[app.stage]}.</p>
        <OfferForm applicationId={app.id} permissions={permissions} onDone={onClose} onCancel={onClose} />
      </section>
    );
  }
  const back = () => {
    if (app.stage !== "offer") onMove({ app, stage: "offer" });
    onClose();
  };
  return (
    <section className="stage-prompt" aria-label={`Placement report for ${app.candidateName}`}>
      <h4>Moving {app.candidateName} to Placed: the placement report</h4>
      <p className="muted small">Saving the report moves them to Placed. Without it they go back to Offer.</p>
      <PlacementForm applicationId={app.id} permissions={permissions} onDone={onClose} onCancel={back} />
    </section>
  );
}
