// A contact's card, like a candidate's: tinted in the colour of their role
// (Settings › Contact roles: hiring manager, HR…), the role down its right
// edge, and whether they still work there. Both are picked here and saved
// straight away.
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError, api, type Contact, type Schemas, unwrap } from "../api/client";
import { contactKey, contactsKey } from "../api/keys";
import { CONTACT_STATUS, keysOf } from "../labels";
import { tint } from "./companyCategory";
import { EmailCheckBadge } from "./EmailCheckBadge";
import { ErrorMessage } from "./ErrorMessage";
import { useFieldSettings } from "./useFieldSettings";

/** The grey of a contact whose role is not set. */
export const NO_ROLE_COLOR = "#6b7280";

type Change = { role?: string | null; status?: Schemas["ContactStatus"] };

export function ContactBanner({ contact: c, canEdit }: { contact: Contact; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const save = useMutation({
    mutationFn: async (change: Change) =>
      unwrap(await api.PATCH("/v1/contacts/{contactId}", { params: { path: { contactId: c.id } }, body: { version: c.version, ...change } })),
    onSuccess: (updated) => {
      queryClient.setQueryData(contactKey(c.id), updated);
      void queryClient.invalidateQueries({ queryKey: contactsKey });
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === "CONFLICT_STALE_VERSION") void queryClient.invalidateQueries({ queryKey: contactKey(c.id) });
    },
  });

  const color = c.role ? fs.color("contact_role", c.role) : NO_ROLE_COLOR;
  const roleLabel = c.role ? fs.label("contact_role", c.role) : "Role not set";
  const left = c.status === "left";

  return (
    <section className={left ? "banner candidate-banner contact-banner has-left" : "banner candidate-banner contact-banner"} aria-label="Contact"
      style={{ background: `linear-gradient(120deg, ${tint(color, 0.72)}, ${tint(color, 0.9)})` }}>
      <div className="banner-body">
        <div className="banner-top">
          <div className="banner-who">
            <div className="row">
              <h2 className="banner-name">{c.fullName}</h2>
              {canEdit ? (
                <select className="banner-status contact-status" aria-label="Still works there?" value={c.status} disabled={save.isPending}
                  onChange={(e) => { save.mutate({ status: e.target.value as Schemas["ContactStatus"] }); }}>
                  {keysOf(CONTACT_STATUS).map((s) => <option key={s} value={s}>{CONTACT_STATUS[s]}</option>)}
                </select>
              ) : <span className="banner-status contact-status">{CONTACT_STATUS[c.status]}</span>}
            </div>
            <p className="banner-job">
              {c.title ? `${c.title} at ` : ""}<a href={`#/companies/${c.companyId}`}>{c.companyName}</a>
            </p>
          </div>
          <dl className="banner-facts">
            <div><dt>LinkedIn</dt><dd>{c.linkedinUrl ? <a href={c.linkedinUrl} target="_blank" rel="noreferrer">{c.linkedinUrl.replace(/^https:\/\/www\.linkedin\.com\/(in\/)?/, "")}</a> : "—"}</dd></div>
          </dl>
        </div>

        {canEdit && (
          <div className="row" role="group" aria-label="Role">
            {fs.options("contact_role", c.role).map((o) => (
              <button key={o.key} type="button" className="banner-pill" aria-pressed={c.role === o.key} disabled={save.isPending}
                onClick={() => { save.mutate({ role: c.role === o.key ? null : o.key }); }}>
                <span className="dot" style={{ background: fs.color("contact_role", o.key), borderColor: fs.color("contact_role", o.key) }} />
                {o.label}
              </button>
            ))}
            <a className="small" href="#/settings#contact-roles">Change the roles</a>
          </div>
        )}
        <ErrorMessage error={save.error} />

        <div className="banner-strip">
          <div>
            <span className="label">Email</span>
            <span className="figure small">{c.email ? <a href={`mailto:${c.email}`}>{c.email}</a> : "—"}</span>
            {c.email && <EmailCheckBadge check={c.emailCheck} />}
          </div>
          <div><span className="label">Mobile</span><span className="figure small">{c.mobile ? <a href={`tel:${c.mobile}`}>{c.mobile}</a> : "—"}</span></div>
          <div><span className="label">Office phone</span><span className="figure small">{c.phone ? <a href={`tel:${c.phone}`}>{c.phone}</a> : "—"}</span></div>
        </div>
      </div>
      <div className="banner-column" style={{ background: color }}>
        <span><span className="visually-hidden">Role: </span>{roleLabel}</span>
      </div>
    </section>
  );
}
