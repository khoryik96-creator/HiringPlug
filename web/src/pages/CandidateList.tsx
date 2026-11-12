import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Fragment, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { candidatesKey } from "../api/keys";
import { CandidateExtras, extrasBody } from "../components/MandatoryExtras";
import { DuplicateEmail } from "../components/DuplicateEmail";
import { ErrorMessage } from "../components/ErrorMessage";
import { Chips } from "../components/LabelInput";
import { categoryLabel, useIndustries } from "../components/IndustryPicker";
import { AdvancedSearch } from "../components/search/AdvancedSearch";
import { activeChips, type CandidateSort, EMPTY, isEmpty, loadSearch, nextPage, type PageAt, saveSearch, type SearchState, snippetParts, SORTS, toBody } from "../components/search/candidateSearch";
import { COLUMNS, ColumnPicker, useCandidateColumns, WaLegend, WaName } from "../components/candidateColumns";
import { pickRowProps } from "../components/pickRow";
import { PickBar, SeparateEmail } from "../components/SeparateEmail";
import { usePicked } from "../components/usePicked";
import { hashParam } from "../hashQuery";

/** Search and add candidates. */
export function CandidateList({ canCreate, canSeeSalary = false, canAiSearch = false, canEmail = false }: { canCreate: boolean; canSeeSalary?: boolean; canAiSearch?: boolean; canEmail?: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [mine, setMine] = useState(false);
  const [adv, setAdv] = useState<SearchState>(loadSearch);
  const [panel, setPanel] = useState(false);
  const [adding, setAdding] = useState(false);
  const cols = useCandidateColumns();
  const pick = usePicked();
  const [emailing, setEmailing] = useState(false);
  const personOf = (c: Schemas["CandidateSummary"]) => ({ id: c.id, person: { candidateId: c.id, name: c.fullName, email: c.email } });
  const advanced = !isEmpty(adv);
  const apply = (s: SearchState) => { setAdv(s); saveSearch(s); };
  const [sortBy, setSortBy] = useState<CandidateSort>(() => {
    const kept = readSort();
    return SORTS.some((s) => s.value === kept && (!s.salary || canSeeSalary)) ? kept : "newest";
  });
  const list = useInfiniteQuery({
    queryKey: [...candidatesKey, q, mine, sortBy],
    enabled: !advanced,
    initialPageParam: undefined as PageAt | undefined,
    queryFn: async ({ pageParam }) => {
      const query: { q?: string; owner?: string; sort?: CandidateSort; cursor?: string; offset?: number } = {};
      if (q.trim()) query.q = q.trim();
      if (mine) query.owner = "me";
      if (sortBy !== "newest") query.sort = sortBy;
      if (pageParam?.cursor) query.cursor = pageParam.cursor;
      if (pageParam?.offset) query.offset = pageParam.offset;
      return unwrap(await api.GET("/v1/candidates", { params: { query } }));
    },
    getNextPageParam: nextPage,
  });
  const found = useInfiniteQuery({
    queryKey: [...candidatesKey, "search", q, mine, adv, sortBy],
    enabled: advanced,
    initialPageParam: undefined as PageAt | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.POST("/v1/candidates/search", { body: toBody(adv, q, mine, pageParam, sortBy) })),
    getNextPageParam: nextPage,
  });
  const shownList = advanced ? found : list;
  const hits: Hit[] = advanced
    ? (found.data?.pages.flatMap((pg) => pg.items) ?? [])
    : (list.data?.pages.flatMap((pg) => pg.items.map((c) => ({ candidate: c, matchedIn: [], cvSnippet: null }))) ?? []);
  const terms = found.data?.pages[0]?.terms ?? [];
  const hl = advanced && adv.boolean.trim() ? `?hl=${encodeURIComponent(adv.boolean.trim())}` : "";
  const industries = useIndustries();
  const chips = activeChips(adv, (k) => cols.look.label("candidate_source", k), (k) => { const item = industries.data?.items.find((i) => i.id === k); return item && industries.data ? categoryLabel(item, industries.data.items) : k.startsWith("industry_") ? "Unavailable category" : k; });
  const width = cols.columns.length + 1;
  const allShown = hits.length > 0 && hits.every((h) => pick.ids.has(h.candidate.id));
  // With email, clicking a row chooses the candidate (the name still opens them).
  const rowProps = (c: (typeof hits)[number]["candidate"], snippet: unknown) => {
    if (!canEmail) return { className: snippet ? "has-snippet" : undefined };
    const chosen = pick.ids.has(c.id);
    const row = pickRowProps(chosen, () => { pick.pick([personOf(c)], !chosen); });
    return { ...row, className: `${row.className}${snippet ? " has-snippet" : ""}` };
  };

  return (
    <main className="candidates-page wide">
      <div className="page-head">
        <h2>Candidates</h2>
        <div className="row">
          {canAiSearch && <a className="doc-btn ai-link" href="#/candidates/ai-search">✦ AI search</a>}
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>+ Add candidate</button>}
        </div>
      </div>
      {canCreate && adding && <AddCandidate onCancel={() => { setAdding(false); }} />}
      <div className="list-toolbar">
        <div className="search-box">
          <span aria-hidden="true">⌕</span>
          <input aria-label="Search candidates" placeholder="Search by name, email, phone, title or company" value={q} onChange={(e) => { setQ(e.target.value); }} />
        </div>
        <label className={`toggle-chip${mine ? " on" : ""}`}><input type="checkbox" checked={mine} onChange={(e) => { setMine(e.target.checked); }} /> Mine only</label>
        <button type="button" className={`doc-btn${advanced ? " primary-soft" : ""}`} aria-expanded={panel} onClick={() => { setPanel(!panel); }}>
          Advanced search{chips.length > 0 ? ` · ${String(chips.length)}` : ""}
        </button>
        <label className="sort-by">
          <span className="visually-hidden">Sort by</span>
          <select aria-label="Sort by" value={sortBy} onChange={(e) => { const v = e.target.value as CandidateSort; setSortBy(v); keepSort(v); }}>
            {SORTS.filter((s) => !s.salary || canSeeSalary).map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
          </select>
        </label>
        <ColumnPicker columns={cols.columns} chosen={cols.chosen} onSave={(c) => { cols.save.mutate(c); }} />
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
      <ErrorMessage error={shownList.error ?? cols.save.error} />
      <div className="list-meta">
        <span role="status">
          {shownList.data && (hits.length === 0 ? "No candidates match."
            : `${String(hits.length)}${shownList.hasNextPage ? "+" : ""} candidate${hits.length === 1 ? "" : "s"}`)}
          {advanced && terms.length > 0 && hits.length > 0 && <> · <mark>marked</mark>: where their CV has {terms.join(", ")}</>}
        </span>
        <WaLegend />
      </div>
      {emailing && <SeparateEmail people={pick.people} onClose={() => { setEmailing(false); pick.clear(); }} />}
      {canEmail && !emailing && <PickBar count={pick.picked.size} onEmail={() => { setEmailing(true); }} onClear={pick.clear} />}
      {canEmail && hits.length > 0 && !emailing && (
        <button type="button" className="link small" onClick={() => { pick.pick(hits.map((h) => personOf(h.candidate)), !allShown); }}>
          {allShown ? "Unselect all shown" : "Select all shown"}
        </button>
      )}
      <div className="table-scroll">
        <table className={canEmail ? "candidates-table pick-table" : "candidates-table"}>
          <thead>
            <tr>
              <th>Name</th>
              {cols.columns.map((col) => <th key={col}>{COLUMNS[col].header}</th>)}
            </tr>
          </thead>
          <tbody>
            {hits.map(({ candidate: c, matchedIn, cvSnippet }) => (
              <Fragment key={c.id}>
                <tr {...rowProps(c, cvSnippet)}>
                  <td className="name-cell">
                    <WaName c={c} href={`#/candidates/${c.id}${hl}`}>
                      {" "}
                      {!cols.columns.includes("tags") && c.tags.length > 0 && <Chips labels={c.tags} />}
                      {matchedIn.length > 0 && <span className="found-in">{matchedIn.includes("cv") ? "in CV" : "in profile"}</span>}
                      {canEmail && pick.ids.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
                    </WaName>
                  </td>
                  {cols.columns.map((col) => <td key={col} className={`col-${col}`}>{COLUMNS[col].cell(c, cols.look)}</td>)}
                </tr>
                {cvSnippet && (
                  <tr className="snippet-row">
                    <td colSpan={width}>
                      <span className="snippet-label">CV</span>…
                      {snippetParts(cvSnippet).map((p, i) => (p.match ? <mark key={i}>{p.text}</mark> : <span key={i}>{p.text}</span>))}
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>
      {shownList.hasNextPage && (
        <button type="button" className="doc-btn" disabled={shownList.isFetchingNextPage} onClick={() => { void shownList.fetchNextPage(); }}>Show more</button>
      )}
    </main>
  );
}

type Hit = Schemas["CandidateSearchHit"];

function AddCandidate({ onCancel }: { onCancel: () => void }) {
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
      className="panel add-candidate"
      aria-label="Add a candidate"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <h3>Add a candidate</h3>
      <div className="add-fields">
        {field("firstName", "First name")}
        {field("lastName", "Last name")}
        {field("email", "Email")}
        {field("phone", "Phone")}
        <CandidateExtras values={extras} onChange={setExtras} />
      </div>
      <div className="row">
        <button type="submit" disabled={!form.firstName.trim() || create.isPending}>Add candidate</button>
        <button type="button" className="link" onClick={onCancel}>Cancel</button>
      </div>
      <ErrorMessage error={create.error}><DuplicateEmail error={create.error} /></ErrorMessage>
    </form>
  );
}

// The chosen order is kept on this device (a convenience only).
const SORT_KEY = "hiringplug.candidates.sort";
function readSort(): CandidateSort {
  try { return (window.localStorage.getItem(SORT_KEY) ?? "newest") as CandidateSort; } catch { return "newest"; }
}
function keepSort(v: CandidateSort) {
  try { window.localStorage.setItem(SORT_KEY, v); } catch { /* only a convenience */ }
}
