// Delete a candidate, contact or company (admins): it goes to the Bin for 30
// days, where an admin can restore it. A company's contacts and jobs move to
// another company you choose, or go to the Bin with it.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { CompanyPicker, type PickedCompany } from "./CompanyPicker";
import { ErrorMessage } from "./ErrorMessage";

type Kind = Schemas["BinKind"];
const LIST: Record<Kind, string> = { candidate: "#/candidates", contact: "#/contacts", company: "#/companies" };

const plural = (n: number, one: string) => `${String(n)} ${one}${n === 1 ? "" : "s"}`;

export function DeleteRecord({ kind, id }: { kind: Kind; id: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" className="danger-quiet" onClick={() => { setOpen(true); }}>Delete</button>
      {open && <DeleteDialog kind={kind} id={id} onClose={() => { setOpen(false); }} />}
    </>
  );
}

function DeleteDialog({ kind, id, onClose }: { kind: Kind; id: string; onClose: () => void }) {
  const queryClient = useQueryClient();
  const check = useQuery({
    queryKey: ["bin-check", kind, id],
    queryFn: async () => unwrap(await api.GET("/v1/bin/check", { params: { query: { kind, id } } })),
    retry: false,
  });
  const [move, setMove] = useState(true);
  const [to, setTo] = useState<PickedCompany | null>(null);
  const c = check.data;
  const linked = c ? c.contacts + c.jobs : 0;
  // A company whose jobs have a placement can still go once they move to another company.
  const mustMove = kind === "company" && !!c?.blocked && linked > 0;
  const moving = kind === "company" && linked > 0 && (move || mustMove);
  const blocked = c?.blocked && !mustMove ? c.blocked : null;
  const remove = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/bin", { body: { kind, id, ...(moving && to ? { moveTo: to.id } : {}) } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries();
      location.hash = LIST[kind];
    },
  });
  const what = c ? [c.contacts > 0 && plural(c.contacts, "contact"), c.jobs > 0 && plural(c.jobs, "job")].filter(Boolean).join(" and ") : "";
  return (
    <div className="dialog-backdrop" role="presentation" onClick={onClose}>
      <div className="dialog delete-dialog" role="dialog" aria-modal="true" aria-label={`Delete ${kind}`} onClick={(e) => { e.stopPropagation(); }}>
        <ErrorMessage error={check.error} />
        {c && <>
          <h3>Delete {c.label}?</h3>
          {blocked
            ? <p className="warning" role="alert">{blocked}</p>
            : <p className="muted">It goes to the Bin for 30 days (Settings › Bin), where an admin can restore it; then it is deleted for good.
              {c.applications > 0 && ` ${plural(c.applications, "pipeline entry").replace("entrys", "entries")} are hidden with it.`}</p>}
          {mustMove && <p className="warning small">Its jobs have a placement, so they move to another company; they cannot be deleted.</p>}
          {!blocked && kind === "company" && linked > 0 && (
            <fieldset className="delete-links">
              <legend>Its {what}</legend>
              <label className={move || mustMove ? "on" : undefined}>
                <input type="radio" name="links" checked={move || mustMove} onChange={() => { setMove(true); }} /> Move them to another company
              </label>
              {(move || mustMove) && <div className="delete-move"><CompanyPicker value={to} onChange={setTo} aria-invalid={!to} /></div>}
              {!mustMove && (
                <label className={!move ? "on" : undefined}>
                  <input type="radio" name="links" checked={!move} onChange={() => { setMove(false); }} /> Delete them with it (to the Bin)
                </label>
              )}
            </fieldset>
          )}
          <ErrorMessage error={remove.error} />
          <div className="row">
            {!blocked && (
              <button type="button" className="danger" disabled={remove.isPending || (moving && (!to || to.id === id))} onClick={() => { remove.mutate(); }}>
                {moving ? "Move them and delete" : "Delete"}
              </button>
            )}
            <button type="button" className="link" onClick={onClose}>Cancel</button>
          </div>
        </>}
      </div>
    </div>
  );
}
