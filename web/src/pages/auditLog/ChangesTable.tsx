// The Changes tab: who changed what, newest first. A row opens to show what
// changed, from -> to; salary and other hidden fields only say they changed.
import { useInfiniteQuery } from "@tanstack/react-query";
import { Fragment, useState } from "react";

import { api, unwrap } from "../../api/client";
import { ErrorMessage } from "../../components/ErrorMessage";
import { type AuditEntry, changeLines, exportedList, recordHref, whatHappened } from "./auditText";
import { auditLogKey, changesQuery, type Filters, PAGE_SIZE } from "./filters";
import { ListHead } from "./FilterBar";
import { fullWhen, whenText } from "./when";

export function ChangesTable({ filters }: { filters: Filters }) {
  const query = changesQuery(filters);
  const list = useInfiniteQuery({
    queryKey: [...auditLogKey, query],
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }) => unwrap(await api.GET("/v1/audit-log", {
      params: { query: { ...query, limit: PAGE_SIZE, ...(pageParam ? { cursor: pageParam } : {}) } },
    })),
    getNextPageParam: (last) => last.nextCursor,
  });
  const [open, setOpen] = useState<Set<string>>(new Set());
  const toggle = (id: string) => {
    setOpen((old) => { const next = new Set(old); if (!next.delete(id)) next.add(id); return next; });
  };
  const rows = list.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <>
      <ListHead shown={list.data ? rows.length : null} more={list.hasNextPage} noun={["entry", "entries"]}
        path="/v1/audit-log/export" body={query} />
      <ErrorMessage error={list.error} />
      {list.isPending && <p className="muted" role="status">Loading…</p>}
      {list.data && rows.length === 0 && (
        <p className="audit-empty">
          {filters.who || filters.what ? "Nothing matches these filters in these days." : "Nothing was changed in these days."} Try other filters or more days.
        </p>
      )}
      {rows.length > 0 && (
        <div className="table-scroll audit-scroll">
          <table className="audit-table audit-changes" aria-label="Changes">
            <thead><tr><th>When</th><th>Who</th><th>What</th><th>Record</th></tr></thead>
            <tbody>
              {rows.map((e) => (
                <Fragment key={e.id}>
                  <tr className={open.has(e.id) ? "audit-row open" : "audit-row"} onClick={() => { toggle(e.id); }}>
                    <td className="audit-when">
                      <button type="button" className="link audit-toggle" aria-expanded={open.has(e.id)} aria-controls={`audit-diff-${e.id}`}
                        title={fullWhen(e.at)} onClick={(ev) => { ev.stopPropagation(); toggle(e.id); }}>
                        <span aria-hidden="true" className="audit-chevron">›</span>{whenText(e.at)}
                      </button>
                    </td>
                    <td className="audit-who">{e.actorKind === "user" ? e.actorName : <span className="muted">{e.actorName}</span>}</td>
                    <td className="audit-what">{whatHappened(e)}</td>
                    <td className="audit-record"><Record entry={e} /></td>
                  </tr>
                  {open.has(e.id) && (
                    <tr className="audit-diff-row"><td colSpan={4} id={`audit-diff-${e.id}`}><Diff entry={e} /></td></tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {list.hasNextPage && (
        <button type="button" className="doc-btn audit-more" disabled={list.isFetchingNextPage} onClick={() => { void list.fetchNextPage(); }}>
          {list.isFetchingNextPage ? "Loading…" : "Load more"}
        </button>
      )}
    </>
  );
}

function Record({ entry }: { entry: AuditEntry }) {
  const list = exportedList(entry.action);
  if (list) return <>{list}</>;
  if (!entry.entityName) return <span className="muted">–</span>;
  const href = recordHref(entry);
  return href ? <a href={href} onClick={(ev) => { ev.stopPropagation(); }}>{entry.entityName}</a> : <>{entry.entityName}</>;
}

function Diff({ entry }: { entry: AuditEntry }) {
  const lines = changeLines(entry.changes);
  if (lines.length === 0) return <div className="audit-diff muted">No details were kept for this entry.</div>;
  return (
    <div className="audit-diff">
      {lines.map((l) => (
        <div key={l.field}>
          <span className="audit-field">{l.field}:</span>{" "}
          {l.kind === "change" && <><s>{l.from}</s> → <ins>{l.to}</ins></>}
          {l.kind === "hidden" && (l.amount
            ? <>changed <span className="muted">(amounts are never kept here)</span></>
            : <span className="muted">not kept here (it may show a salary)</span>)}
          {l.kind === "detail" && l.value}
        </div>
      ))}
    </div>
  );
}
