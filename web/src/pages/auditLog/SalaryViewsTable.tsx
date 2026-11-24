// The Salary views tab: who was shown a candidate's salary, one line per
// person, kind, candidate and day, with how many times that day. A list,
// search or export is one line with how many candidates it showed. No amounts.
import { useInfiniteQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { ErrorMessage } from "../../components/ErrorMessage";
import { type Filters, PAGE_SIZE, SALARY_KIND, salaryQuery, salaryViewsKey } from "./filters";
import { ListHead } from "./FilterBar";
import { dayText, fullWhen, timeText } from "./when";

type SalaryView = Schemas["SalaryView"];

export function SalaryViewsTable({ filters }: { filters: Filters }) {
  const query = salaryQuery(filters);
  const list = useInfiniteQuery({
    queryKey: [...salaryViewsKey, query],
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }) => unwrap(await api.GET("/v1/salary-views", {
      params: { query: { ...query, limit: PAGE_SIZE, ...(pageParam ? { cursor: pageParam } : {}) } },
    })),
    getNextPageParam: (last) => last.nextCursor,
  });
  const rows = list.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <>
      <ListHead shown={list.data ? rows.length : null} more={list.hasNextPage} noun={["line", "lines"]}
        path="/v1/salary-views/export" body={query} />
      <ErrorMessage error={list.error} />
      {list.isPending && <p className="muted" role="status">Loading…</p>}
      {list.data && rows.length === 0 && (
        <p className="audit-empty">
          {filters.who || filters.what ? "No salary was shown that matches these filters in these days." : "No one was shown a salary in these days."} Try
          other filters or more days.
        </p>
      )}
      {rows.length > 0 && (
        <div className="table-scroll">
          <table className="audit-table" aria-label="Salary views">
            <thead><tr><th>When</th><th>Who</th><th>Saw</th><th>Candidate</th><th className="num">Times that day</th></tr></thead>
            <tbody>
              {rows.map((v) => (
                <tr key={v.id}>
                  <td className="audit-when" title={v.count > 1 ? `First ${fullWhen(v.firstAt)}, last ${fullWhen(v.lastAt)}` : fullWhen(v.lastAt)}>
                    {dayText(v.day)} {timeText(v.lastAt)}
                  </td>
                  <td>{v.userName}</td>
                  <td>{SALARY_KIND[v.kind]}</td>
                  <td><Candidate view={v} /></td>
                  <td className="num">{v.count.toLocaleString("en")}</td>
                </tr>
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

/** The candidate whose salary it was, or "48 candidates" for a list, search or export. */
function Candidate({ view: v }: { view: SalaryView }) {
  if (v.candidateId && v.candidateName) return <a href={`#/candidates/${v.candidateId}`}>{v.candidateName}</a>;
  if (v.candidateId) return <span className="muted">A candidate</span>;
  return <>{v.candidates.toLocaleString("en")} candidate{v.candidates === 1 ? "" : "s"}</>;
}
