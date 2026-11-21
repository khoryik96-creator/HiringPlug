import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { formatDay, keysOf, LEAD_SOURCE_KIND, LEAD_STATUS } from "../../labels";
import { CloseIcon, IconButton, PersonIcon, StarIcon } from "../ActionIcons";
import { ErrorMessage } from "../ErrorMessage";

type Lead = Schemas["JobLead"];

function adLink(raw: string | null) {
  if (!raw) return null;
  try {
    const url = new URL(raw);
    return ["https:", "http:"].includes(url.protocol) && !url.username && !url.password ? url.href : null;
  } catch { return null; }
}

export function LeadList({ items, canManage, onReview }: { items: Lead[]; canManage: boolean; onReview: (id: string) => void }) {
  return <div className="leads-scroll">
    <table>
      <thead><tr><th scope="col">Company</th><th scope="col">Job</th><th scope="col">Location</th>
        <th scope="col">Seen</th><th scope="col">Status</th><th scope="col">Actions</th></tr></thead>
      <tbody>{items.map((lead) => <LeadRow key={lead.id} lead={lead} canManage={canManage} onReview={() => { onReview(lead.id); }} />)}</tbody>
    </table>
  </div>;
}

function LeadRow({ lead, canManage, onReview }: { lead: Lead; canManage: boolean; onReview: () => void }) {
  const queryClient = useQueryClient();
  const setStatus = useMutation({
    mutationFn: async (status: Schemas["LeadStatus"]) =>
      unwrap(await api.PATCH("/v1/job-leads/{jobLeadId}", { params: { path: { jobLeadId: lead.id } }, body: { version: lead.version, status } })),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["job-leads"] }),
  });
  const link = adLink(lead.url);
  return <tr>
    <td>{lead.companyId ? <a href={`#/companies/${lead.companyId}`}>{lead.companyName}</a> : lead.companyName}</td>
    <td>{link ? <a href={link} target="_blank" rel="noreferrer noopener">{lead.title}</a> : lead.title}</td>
    <td>{lead.location}</td>
    <td className="muted">{formatDay((lead.postedAt ?? lead.firstSeenAt).slice(0, 10))}{lead.closedAt && <> · closed</>}</td>
    <td>
      {canManage ? (
        <select aria-label={`Status of ${lead.title} at ${lead.companyName}`} value={lead.status} disabled={setStatus.isPending}
          onChange={(e) => { setStatus.mutate(e.target.value as Schemas["LeadStatus"]); }}>
          {keysOf(LEAD_STATUS).map((s) => <option key={s} value={s}>{LEAD_STATUS[s]}</option>)}
        </select>
      ) : LEAD_STATUS[lead.status]}
      <ErrorMessage error={setStatus.error} />
    </td>
    <td><div className="row"><Favorite id={lead.id} active={!!lead.favorite} />
      <button type="button" className="link" onClick={onReview} aria-label={`Review ${lead.title} at ${lead.companyName}`}>Review</button></div></td>
  </tr>;
}

function Favorite({ id, active }: { id: string; active: boolean }) {
  const queryClient = useQueryClient();
  const set = useMutation({
    mutationFn: async () => {
      unwrap(await api.PUT("/v1/job-leads/{jobLeadId}/favorite", {
        params: { path: { jobLeadId: id } }, body: { favorite: !active },
      }));
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["job-leads"] }),
  });
  return <>
    <IconButton icon={<StarIcon />} pressed={active} disabled={set.isPending}
      label={active ? "Remove from favorites" : "Add to favorites"} onClick={() => { set.mutate(); }} />
    <ErrorMessage error={set.error} />
  </>;
}

export function LeadReview({ id, canSeeCompany, onClose }: { id: string; canSeeCompany: boolean; onClose: () => void }) {
  const lead = useQuery({
    queryKey: ["job-leads", "detail", id],
    queryFn: async ({ signal }) => unwrap(await api.GET("/v1/job-leads/{jobLeadId}", { signal, params: { path: { jobLeadId: id } } })),
  });
  const item = lead.data;
  const link = item ? adLink(item.url) : null;
  return <aside className="panel leads-review" aria-label="Lead details">
    <div className="row spread">
      <h3>{item?.title ?? "Lead details"}</h3>
      <IconButton label="Close lead details" icon={<CloseIcon />} onClick={onClose} />
    </div>
    <ErrorMessage error={lead.error} />
    {item && <>
      <p>{item.companyName}{item.location ? ` · ${item.location}` : ""}</p>
      <p className="muted small">{item.sourceKind ? LEAD_SOURCE_KIND[item.sourceKind] : "Saved by hand"} ·
        {item.closedAt ? " Listing closed" : ` Last seen ${formatDay(item.lastSeenAt.slice(0, 10))}`}</p>
      {item.description && <details open>
        <summary>Job description</summary>
        <p className="pre">{item.description}</p>
        {item.descriptionObservedAt && <small className="muted">Read {formatDay(item.descriptionObservedAt.slice(0, 10))}</small>}
      </details>}
      {item.notes && <p className="pre">{item.notes}</p>}
      <div className="row">
        {link && <a href={link} target="_blank" rel="noreferrer noopener">Open job ad ↗</a>}
        <Favorite id={id} active={!!item.favorite} />
      </div>
      {canSeeCompany && item.companyId && <a className="row" href={`#/companies/${item.companyId}?tab=find-hr`}>
        <PersonIcon /> Find HR at {item.companyName}
      </a>}
    </>}
  </aside>;
}
