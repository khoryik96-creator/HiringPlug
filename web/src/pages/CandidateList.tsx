import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Fragment, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { candidatesKey } from "../api/keys";
import { CandidateExtras, extrasBody } from "../components/MandatoryExtras";
import { DuplicateEmail } from "../components/DuplicateEmail";
import { ErrorMessage } from "../components/ErrorMessage";
import { Chips } from "../components/LabelInput";
import { AdvancedSearch } from "../components/search/AdvancedSearch";
import { activeChips, EMPTY, isEmpty, loadSearch, saveSearch, type SearchState, snippetParts, toBody } from "../components/search/candidateSearch";
import { COLUMNS, ColumnPicker, DEFAULT_COLUMNS, useCandidateColumns, WaBar, WaLegend } from "../components/candidateColumns";
import { hashParam } from "../hashQuery";

/** Search and add candidates. */
export function CandidateList({ canCreate, canSeeSalary = false }: { canCreate: boolean; canSeeSalary?: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [mine, setMine] = useState(false);
  const [adv, setAdv] = useState<SearchState>(loadSearch);
  const [panel, setPanel] = useState(false);
  const cols = useCandidateColumns();
  const advanced = !isEmpty(adv);
  const apply = (s: SearchState) => { setAdv(s); saveSearch(s); };
  const list = useQuery({
    queryKey: [...candidatesKey, q, mine],
    enabled: !advanced,
    queryFn: async () => {
      const query: { q?: string; owner?: string } = {};
      if (q.trim()) query.q = q.trim();
      if (mine) query.owner = "me";
      return unwrap(await api.GET("/v1/candidates", { params: { query } }));
    },
  });
  const found = useInfiniteQuery({
    queryKey: [...candidatesKey, "search", q, mine, adv],
    enabled: advanced,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.POST("/v1/candidates/search", { body: toBody(adv, q, mine, pageParam) })),
    getNextPageParam: (last) => last.nextCursor ?? undefined,
  });
  const hits: Hit[] = advanced
    ? (found.data?.pages.flatMap((pg) => pg.items) ?? [])
    : (list.data?.items.map((c) => ({ candidate: c, matchedIn: [], cvSnippet: null })) ?? []);
  const terms = found.data?.pages[0]?.terms ?? [];
  const hl = advanced && adv.boolean.trim() ? `?hl=${encodeURIComponent(adv.boolean.trim())}` : "";
  const chips = activeChips(adv, (k) => cols.look.label("candidate_source", k));
  const width = cols.columns.length + 1;

  return (
    <main className={cols.columns.length > DEFAULT_COLUMNS.length ? "wide" : undefined}>
      <div className="row spread">
        <h2>Candidates</h2>
        <div className="row">
          <input aria-label="Search candidates" placeholder="Name, email, phone, title…" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <label className="small"><input type="checkbox" checked={mine} onChange={(e) => { setMine(e.target.checked); }} /> Mine</label>
          <button type="button" className={`doc-btn${advanced ? " primary-soft" : ""}`} aria-expanded={panel} onClick={() => { setPanel(!panel); }}>
            Advanced search{chips.length > 0 ? ` · ${String(chips.length)}` : ""}
          </button>
          <ColumnPicker columns={cols.columns} chosen={cols.chosen} onSave={(c) => { cols.save.mutate(c); }} />
        </div>
      </div>
      {panel && <AdvancedSearch value={adv} canSeeSalary={canSeeSalary} onSearch={(s) => { apply(s); setPanel(false); }} onClose={() => { setPanel(false); }} />}
      {!panel && chips.length > 0 && (
        <div className="active-filters" aria-label="Search filters">
          {chips.map((c) => (
            <span key={c.label} className="filter-chip">
              {c.label}
              <button type="button" aria-label={`Remove ${c.label}`} onClick={() => { apply(c.clear(adv)); }}>×</button>
            </span>
          ))}
          <button type="button" className="link small" onClick={() => { apply(EMPTY); }}>Clear all</button>
        </div>
      )}
      {canCreate && !panel && !advanced && <AddCandidate />}
      <ErrorMessage error={(advanced ? found.error : list.error) ?? cols.save.error} />
      {advanced && found.data && (
        <p className="muted small" role="status">
          {hits.length === 0 ? "No candidates match." : `${String(hits.length)}${found.hasNextPage ? "+" : ""} found`}
          {terms.length > 0 && hits.length > 0 && <> · <mark>marked</mark> where their CV has {terms.join(", ")}</>}
        </p>
      )}
      <WaLegend />
      <div className="table-scroll">
        <table className="candidates-table">
          <thead>
            <tr>
              <th>Name</th>
              {cols.columns.map((col) => <th key={col}>{COLUMNS[col].header}</th>)}
            </tr>
          </thead>
          <tbody>
            {hits.map(({ candidate: c, matchedIn, cvSnippet }) => (
              <Fragment key={c.id}>
                <tr className={cvSnippet ? "has-snippet" : undefined}>
                  <td className="name-cell">
                    <WaBar c={c} />
                    <a href={`#/candidates/${c.id}${hl}`}>{c.fullName}</a>{" "}
                    {!cols.columns.includes("tags") && c.tags.length > 0 && <Chips labels={c.tags} />}
                    {matchedIn.length > 0 && <span className="found-in">{matchedIn.includes("cv") ? "in CV" : "in profile"}</span>}
                  </td>
                  {cols.columns.map((col) => <td key={col} className={`col-${col}`}>{COLUMNS[col].cell(c, cols.look)}</td>)}
                </tr>
                {cvSnippet && (
                  <tr className="snippet-row">
                    <td colSpan={width}>
                      <span className="snippet-label">CV</span>
                      {snippetParts(cvSnippet).map((p, i) => (p.match ? <mark key={i}>{p.text}</mark> : <span key={i}>{p.text}</span>))}
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>
      {advanced && found.hasNextPage && (
        <button type="button" className="doc-btn" disabled={found.isFetchingNextPage} onClick={() => { void found.fetchNextPage(); }}>Show more</button>
      )}
    </main>
  );
}

type Hit = Schemas["CandidateSearchHit"];

function AddCandidate() {
  const queryClient = useQueryClient();
  const [form, setForm] = useState({ firstName: "", lastName: "", email: "", phone: "" });
  const [extras, setExtras] = useState<Record<string, string>>({});
  const create = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/candidates", {
          body: {
            firstName: form.firstName,
            lastName: form.lastName,
            email: form.email || null,
            phone: form.phone || null,
            ...extrasBody(extras),
          },
        }),
      ),
    onSuccess: (c) => {
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
      window.location.hash = `#/candidates/${c.id}`;
    },
  });
  const field = (name: keyof typeof form, label: string) => (
    <input
      aria-label={label}
      placeholder={label}
      value={form[name]}
      onChange={(e) => { setForm({ ...form, [name]: e.target.value }); }}
    />
  );
  return (
    <form
      className="row panel"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      {field("firstName", "First name")}
      {field("lastName", "Last name")}
      {field("email", "Email")}
      {field("phone", "Phone")}
      <CandidateExtras values={extras} onChange={setExtras} />
      <button type="submit" disabled={!form.firstName.trim() || create.isPending}>
        Add candidate
      </button>
      <ErrorMessage error={create.error}><DuplicateEmail error={create.error} /></ErrorMessage>
    </form>
  );
}
