import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { applicationOffersKey, offersKey } from "../api/keys";
import { OFFER_STATUS, PAY_PERIOD, formatDay, formatMoney, keysOf } from "../labels";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { CURRENCIES, fieldErrors } from "./forms";
import { SaveError } from "./SaveError";

type Offer = Schemas["Offer"];
type Step = Schemas["OfferStep"];

const open = (o: Offer) => o.status !== "declined" && o.status !== "withdrawn";

/** An application's offers (the open one first), or making one. */
export function ApplicationOffers({ applicationId, stage, permissions }: { applicationId: string; stage: string; permissions: Permission[] }) {
  const [making, setMaking] = useState(false);
  const shown = ["submitted", "interview", "offer", "placed"].includes(stage);
  const list = useQuery({
    queryKey: applicationOffersKey(applicationId),
    queryFn: async () => unwrap(await api.GET("/v1/offers", { params: { query: { applicationId } } })).items,
    enabled: shown,
  });
  if (!shown) return null;
  const current = list.data?.find(open);
  return (
    <div className="offers">
      <ErrorMessage error={list.error} />
      {current && <OfferItem offer={current} permissions={permissions} />}
      {making && <OfferForm applicationId={applicationId} permissions={permissions} onDone={() => { setMaking(false); }} />}
      {!current && !making && list.isSuccess && stage !== "placed" && permissions.includes("offers:manage") && (
        <button type="button" className="link" onClick={() => { setMaking(true); }}>Make offer</button>
      )}
    </div>
  );
}

function salaryText(o: Offer): string | null {
  if (!o.salary) return null;
  return `${formatMoney(o.salary.amount, o.salary.currency)} a ${PAY_PERIOD[o.salary.period].toLowerCase()}`;
}

/** One offer: its terms, where it stands, and what the user may do next. */
export function OfferItem({ offer: o, permissions, showCandidate = false }: { offer: Offer; permissions: Permission[]; showCandidate?: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [note, setNote] = useState("");
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: offersKey });
    await queryClient.invalidateQueries({ queryKey: ["applications"] });
  };
  const step = useMutation({
    mutationFn: async (s: Step) =>
      unwrap(await api.POST("/v1/offers/{offerId}/steps", { params: { path: { offerId: o.id } }, body: { version: o.version, step: s } })),
    onSettled: refresh,
  });
  const decide = useMutation({
    mutationFn: async (approve: boolean) => unwrap(await api.POST("/v1/offers/{offerId}/decision", {
      params: { path: { offerId: o.id } }, body: { version: o.version, approve, ...(note.trim() ? { note: note.trim() } : {}) },
    })),
    onSettled: refresh,
  });
  if (editing) return <OfferForm applicationId={o.applicationId} offer={o} permissions={permissions} onDone={() => { setEditing(false); }} />;

  const manage = permissions.includes("offers:manage");
  const approver = permissions.includes("offers:approve");
  const button = (label: string, onClick: () => void) => (
    <button type="button" className="link" disabled={step.isPending || decide.isPending} onClick={onClick}>{label}</button>
  );
  const salary = salaryText(o);
  return (
    <article className="offer" aria-label={`Offer to ${o.candidateName}`}>
      <div>
        <strong>Offer</strong>{showCandidate && <> to <a href={`#/candidates/${o.candidateId}`}>{o.candidateName}</a> for {o.jobTitle}</>}{" "}
        <span className={`badge ${o.status}`}>{OFFER_STATUS[o.status]}</span>{" "}
        <span className="muted">
          {salary ?? (o.restrictedFields.includes("salary") ? "" : "no salary yet")}
          {o.startDate && <> · starts {formatDay(o.startDate)}</>}
          {o.expiresOn && <> · answer by {formatDay(o.expiresOn)}</>}
        </span>
      </div>
      {o.terms && <p className="muted">{o.terms}</p>}
      {o.status === "pending_approval" && o.requestedByName && <p className="muted">Approval asked by {o.requestedByName}.</p>}
      {o.decidedByName && o.status !== "pending_approval" && (
        <p className="muted">
          {o.status === "draft" ? "Sent back" : "Approved"} by {o.decidedByName}{o.decisionNote && <>: {o.decisionNote}</>}
        </p>
      )}
      {o.status === "accepted" && <p>Accepted. Record the placement when the start is confirmed.</p>}
      <div className="row">
        {manage && ["draft", "pending_approval", "approved"].includes(o.status) && button("Change", () => { setEditing(true); })}
        {manage && o.status === "draft" && !approver && button("Ask for approval", () => { step.mutate("request_approval"); })}
        {manage && (o.status === "approved" || (o.status === "draft" && approver)) && button("Mark as sent", () => { step.mutate("send"); })}
        {manage && o.status === "sent" && (
          <>
            {button("Accepted", () => { step.mutate("accept"); })}
            {button("Declined", () => { step.mutate("decline"); })}
          </>
        )}
        {manage && ["draft", "pending_approval", "approved", "sent"].includes(o.status) && button("Withdraw", () => { step.mutate("withdraw"); })}
      </div>
      {approver && o.status === "pending_approval" && o.requestedBy !== me.data?.user.id && (
        <div className="row" role="group" aria-label="Your decision">
          <input aria-label="Note" placeholder="Note" value={note} onChange={(e) => { setNote(e.target.value); }} />
          <button type="button" disabled={decide.isPending} onClick={() => { decide.mutate(true); }}>Approve</button>
          <button type="button" className="link" disabled={decide.isPending} onClick={() => { decide.mutate(false); }}>Send back</button>
        </div>
      )}
      <ErrorMessage error={step.error ?? decide.error} />
    </article>
  );
}

/** Draft an offer, or change its terms. */
function OfferForm({ applicationId, offer, permissions, onDone }: { applicationId: string; offer?: Offer; permissions: Permission[]; onDone: () => void }) {
  const queryClient = useQueryClient();
  const withSalary = permissions.includes("offers.salary:update");
  const [opened] = useState(() => ({ version: offer?.version ?? 0 }));
  const [form, setForm] = useState({
    amount: offer?.salary?.amount ?? "", currency: offer?.salary?.currency ?? "MYR", period: offer?.salary?.period ?? "month",
    startDate: offer?.startDate ?? "", expiresOn: offer?.expiresOn ?? "", terms: offer?.terms ?? "",
  });
  const save = useMutation({
    mutationFn: async () => {
      const amount = form.amount.replace(/[,\s]/g, "");
      const salary = amount ? { amount, currency: form.currency.trim().toUpperCase(), period: form.period } : null;
      if (offer) {
        return unwrap(await api.PATCH("/v1/offers/{offerId}", {
          params: { path: { offerId: offer.id } },
          body: {
            version: opened.version, startDate: form.startDate || null, expiresOn: form.expiresOn || null, terms: form.terms.trim() || null,
            ...(withSalary ? { salary } : {}),
          },
        }));
      }
      return unwrap(await api.POST("/v1/applications/{applicationId}/offers", {
        params: { path: { applicationId } },
        body: {
          ...(form.startDate ? { startDate: form.startDate } : {}), ...(form.expiresOn ? { expiresOn: form.expiresOn } : {}),
          ...(form.terms.trim() ? { terms: form.terms.trim() } : {}), ...(withSalary && salary ? { salary } : {}),
        },
      }));
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: offersKey });
      await queryClient.invalidateQueries({ queryKey: ["applications"] }); // the stage moves to offer
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = (key: keyof typeof form) => (e: { target: { value: string } }) => { setForm((f) => ({ ...f, [key]: e.target.value })); };
  return (
    <form className="panel form-grid" aria-label={offer ? "Change offer" : "Make offer"} onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      {withSalary && (
        <>
          <datalist id="offer-currencies">{CURRENCIES.map((c) => <option key={c} value={c} />)}</datalist>
          <Field label="Salary" error={errors["salary.amount"]}><input inputMode="decimal" value={form.amount} onChange={set("amount")} /></Field>
          <Field label="Currency"><input list="offer-currencies" className="code" value={form.currency} onChange={set("currency")} /></Field>
          <Field label="Per">
            <select value={form.period} onChange={set("period")}>
              {keysOf(PAY_PERIOD).map((p) => <option key={p} value={p}>{PAY_PERIOD[p]}</option>)}
            </select>
          </Field>
        </>
      )}
      <Field label="Start date"><input type="date" value={form.startDate} onChange={set("startDate")} /></Field>
      <Field label="Answer by"><input type="date" value={form.expiresOn} onChange={set("expiresOn")} /></Field>
      <label className="field full">Other terms<textarea rows={3} value={form.terms} onChange={set("terms")} /></label>
      {offer && ["pending_approval", "approved"].includes(offer.status) && (
        <p className="full warning">Changing it sends the offer back to draft: it needs approval again.</p>
      )}
      <SaveError error={save.error} shown={["salary.amount"]} onReload={onDone} />
      <div className="row full">
        <button type="submit" disabled={save.isPending}>{offer ? "Save" : "Draft offer"}</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
    </form>
  );
}

/** The dashboard's offers waiting for the user's approval. */
export function OffersToApprove({ permissions }: { permissions: Permission[] }) {
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const list = useQuery({
    queryKey: [...offersKey, "pending_approval"],
    queryFn: async () => unwrap(await api.GET("/v1/offers", { params: { query: { status: "pending_approval" } } })).items,
  });
  const mine = list.data?.filter((o) => o.requestedBy !== me.data?.user.id) ?? [];
  if (mine.length === 0) return <ErrorMessage error={list.error} />;
  return (
    <section className="panel" aria-label="Offers to approve">
      <h3>Offers to approve</h3>
      {mine.map((o) => <OfferItem key={o.id} offer={o} permissions={permissions} showCandidate />)}
    </section>
  );
}
