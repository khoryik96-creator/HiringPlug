import { useInfiniteQuery } from "@tanstack/react-query";
import { Fragment, useCallback, useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { candidatesKey } from "../api/keys";
import { CvImports } from "../components/CvImports";
import { CV_TYPES } from "../components/DropZone";
import { DropHint, usePageDrop } from "../components/PageDrop";
import { BooleanSearchBox, ListSearchError, looksBoolean } from "../components/search/BooleanSearchBox";
import { InFolders, useFolderView, useInFolders, WithFolders } from "../components/folders/FolderView";
import { Chips } from "../components/LabelInput";
import { categoryLabel, useIndustries } from "../components/IndustryPicker";
import { AdvancedSearch } from "../components/search/AdvancedSearch";
import { OpenAsSwitch, useOpenAs } from "../components/search/OpenAs";
import { ResumeReader } from "../components/search/ResumeReader";
import { activeChips, type CandidateSort, EMPTY, isEmpty, loadSearch, nextPage, type PageAt, saveSearch, type SearchState, snippetParts, flipSort, SORTABLE, sortOf, toBody } from "../components/search/candidateSearch";
import { type Column, COLUMNS, ColumnPicker, useCandidateColumns, WaName } from "../components/candidateColumns";
import { useRowPicking } from "../components/useRowPicking";
import { PickBar, SeparateEmail } from "../components/SeparateEmail";
import { PinnedScroll } from "../components/StickyScrollbar";
import { usePicked } from "../components/usePicked";
import { type ColumnMove, useColumnMove } from "../components/useColumnMove";
import { type ColumnWidths, useColumnWidths } from "../components/useColumnWidths";
import { hashParam } from "../hashQuery";

/** Search and add candidates. */
export function CandidateList({ canSeeSalary = false, canAiSearch = false, canEmail = false, canAddToJob = false, canFolders = false, canImport = false, meId = "" }: {
  canSeeSalary?: boolean; canAiSearch?: boolean; canEmail?: boolean; canAddToJob?: boolean; canFolders?: boolean;
  /** May add candidates from CVs (the drop box). */
  canImport?: boolean;
  meId?: string;
}) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [mine, setMine] = useState(false);
  const [adv, setAdv] = useState<SearchState>(loadSearch);
  const [panel, setPanel] = useState(false);
  const [dropped, setDropped] = useState<{ files: File[]; at: number } | null>(null); // CVs dropped on the page
  const drop = usePageDrop(CV_TYPES, (files) => { setDropped({ files, at: Date.now() }); }, canImport);
  const cols = useCandidateColumns();
  const pick = usePicked();
  const folders = useFolderView("candidate", canFolders, meId, pick.clear);
  const folder = folders.filter;
  const canPick = canEmail || canFolders; // rows are chosen to email or to put in a folder
  const [emailing, setEmailing] = useState(false);
  const [openAs, setOpenAs] = useOpenAs();
  const [seen, setSeen] = useState<ReadonlySet<string>>(new Set()); // resumes read, while on this page
  const markSeen = useCallback((id: string) => { setSeen((s) => (s.has(id) ? s : new Set(s).add(id))); }, []);
  const personOf = (c: Schemas["CandidateSummary"]) => ({ id: c.id, person: { candidateId: c.id, name: c.fullName, email: c.email } });
  // Boolean rules typed in the search box (AND, OR, NOT, "…", word*) run the boolean search of CVs and profiles.
  const boolQ = looksBoolean(q);
  const effective: SearchState = boolQ ? { ...adv, boolean: adv.boolean.trim() ? `(${adv.boolean.trim()}) AND (${q.trim()})` : q.trim() } : adv;
  const plainQ = boolQ ? "" : q;
  const advanced = !isEmpty(effective);
  const apply = (s: SearchState) => { setAdv(s); saveSearch(s); };
  const [sortBy, setSortBy] = useState<CandidateSort>(() => {
    const kept = readSort();
    return Object.values(SORTABLE).some((s) => (s.asc === kept || s.desc === kept) && (!s.salary || canSeeSalary)) ? kept : "newest";
  });
  const sortList = (v: CandidateSort) => { setSortBy(v); keepSort(v); };
  const list = useInfiniteQuery({
    queryKey: [...candidatesKey, plainQ, mine, sortBy, folder],
    enabled: !advanced && folders.ready,
    initialPageParam: undefined as PageAt | undefined,
    queryFn: async ({ pageParam }) => {
      const query: { q?: string; owner?: string; sort?: CandidateSort; cursor?: string; offset?: number; folder?: string } = {};
      if (plainQ.trim()) query.q = plainQ.trim();
      if (folder) query.folder = folder;
      if (mine) query.owner = "me";
      if (sortBy !== "newest") query.sort = sortBy;
      if (pageParam?.cursor) query.cursor = pageParam.cursor;
      if (pageParam?.offset) query.offset = pageParam.offset;
      return unwrap(await api.GET("/v1/candidates", { params: { query } }));
    },
    getNextPageParam: nextPage,
  });
  const found = useInfiniteQuery({
    queryKey: [...candidatesKey, "search", plainQ, mine, effective, sortBy, folder],
    enabled: advanced && folders.ready,
    initialPageParam: undefined as PageAt | undefined,
    queryFn: async ({ pageParam }) => unwrap(await api.POST("/v1/candidates/search", { body: toBody(effective, plainQ, mine, pageParam, sortBy, folder) })),
    getNextPageParam: nextPage,
  });
  const shownList = advanced ? found : list;
  const allHits: Hit[] = advanced
    ? (found.data?.pages.flatMap((pg) => pg.items) ?? [])
    : (list.data?.pages.flatMap((pg) => pg.items.map((c) => ({ candidate: c, matchedIn: [], cvSnippet: null }))) ?? []);
  // A sorted list pages by position: if a record moved between pages, it is shown once.
  const hits = allHits.filter((h, i) => allHits.findIndex((x) => x.candidate.id === h.candidate.id) === i);
  const inFolders = useInFolders(folders, hits.map((h) => h.candidate.id));
  const terms = found.data?.pages[0]?.terms ?? [];
  const hl = advanced && effective.boolean.trim() ? `?hl=${encodeURIComponent(effective.boolean.trim())}` : "";
  const industries = useIndustries();
  const chips = activeChips(adv, (k) => cols.look.label("candidate_source", k), (k) => { const item = industries.data?.items.find((i) => i.id === k); return item && industries.data ? categoryLabel(item, industries.data.items) : k.startsWith("industry_") ? "Unavailable category" : k; });
  const filtersOn = chips.length + (mine ? 1 : 0); // on the Filters button
  const width = cols.columns.length + (folders.hasFolders ? 2 : 1);
  const tableRef = useRef<HTMLTableElement>(null);
  const widths = useColumnWidths("candidates", ["name", ...cols.columns, ...(folders.hasFolders ? ["folders"] : [])], tableRef);
  // The columns after the name move by dragging their headings; the order is kept with the choice of columns.
  const moveColumn = useColumnMove(tableRef, cols.columns, (next) => { cols.save.mutate(next); });
  const allShown = hits.length > 0 && hits.every((h) => pick.ids.has(h.candidate.id));
  // With email, clicking a row chooses the candidate (the name still opens them).
  // Shift+click, Ctrl/⌘+click, Ctrl/⌘+A, Esc, ↑↓ (components/useRowPicking.ts).
  const picking = useRowPicking({
    ids: hits.map((h) => h.candidate.id), chosen: pick.ids, enabled: canPick, clear: pick.clear,
    set: (ids, on) => { pick.pick(hits.filter((h) => ids.includes(h.candidate.id)).map((h) => personOf(h.candidate)), on); },
  });
  const rowProps = (c: (typeof hits)[number]["candidate"], snippet: unknown) => {
    if (!canPick) return { className: snippet ? "has-snippet" : undefined };
    const row = picking.rowProps(c.id);
    return { ...row, className: `${row.className ?? ""}${snippet ? " has-snippet" : ""}` };
  };

  const more = shownList.hasNextPage && (
    <button type="button" className="doc-btn" disabled={shownList.isFetchingNextPage} onClick={() => { void shownList.fetchNextPage(); }}>Show more</button>
  );

  return (
    <main className="candidates-page wide" {...drop.props}>
      {drop.overlay}
      <h2 className="visually-hidden">Candidates</h2>
      {canImport && dropped && (
        <div className="dropped-cvs">
          <CvImports key={dropped.at} foldedHistory initialFiles={dropped.files} initialSource="" />
          <button type="button" className="link small" onClick={() => { setDropped(null); }}>Close</button>
        </div>
      )}
      <div className="list-top">
        <BooleanSearchBox value={q} onChange={setQ} label="Search candidates" placeholder="Search by name, email, phone, title or company, or AND, OR, NOT…">
          <button type="button" className={`search-pill${filtersOn > 0 ? " on" : ""}`} aria-expanded={panel} title="Filters (and Mine only)"
            aria-label={`Filters${filtersOn > 0 ? `, ${String(filtersOn)} on` : ""}`} onClick={() => { setPanel(!panel); }}>
            <FunnelIcon />{filtersOn > 0 && <span className="badge">{filtersOn}</span>}
          </button>
          {canAiSearch && <a className="search-pill" href="#/candidates/ai-search">✦ AI</a>}
        </BooleanSearchBox>
        {canImport && <DropHint label="Drop CVs anywhere to upload" skipped={drop.skipped} />}
      </div>
      {panel && <AdvancedSearch value={adv} canSeeSalary={canSeeSalary} mine={{ on: mine, set: setMine }} onSearch={(s) => { apply(s); setPanel(false); }} onClose={() => { setPanel(false); }} />}
      {!panel && filtersOn > 0 && (
        <div className="active-filters" aria-label="Search filters">
          {mine && (
            <span className="filter-chip">
              Mine only
              <button type="button" aria-label="Remove Mine only" onClick={() => { setMine(false); }}>×</button>
            </span>
          )}
          {chips.map((c) => (
            <span key={c.label} className="filter-chip">
              {c.label}
              <button type="button" aria-label={`Remove ${c.label}`} onClick={() => { apply(c.clear(adv)); }}>×</button>
            </span>
          ))}
          <button type="button" className="link small" onClick={() => { apply(EMPTY); setMine(false); }}>Clear all</button>
        </div>
      )}
      <ListSearchError error={shownList.error ?? cols.save.error} />
      <WithFolders view={folders} tools={<>
        {openAs === "profile" && <ColumnPicker columns={cols.columns} chosen={cols.chosen} onSave={(c) => { cols.save.mutate(c); }} />}
        <OpenAsSwitch compact value={openAs} onChange={setOpenAs} />
      </>}>
      <div className="list-meta">
        <span role="status">
          {shownList.data && (hits.length === 0 ? (folder && !q.trim() && !advanced && !mine ? "Nothing in this folder yet: choose candidates under All candidates, then Add to folder (or drag them onto it)." : "No candidates match.")
            : `${String(hits.length)}${shownList.hasNextPage ? "+" : ""} candidate${hits.length === 1 ? "" : "s"}`)}
          {advanced && terms.length > 0 && hits.length > 0 && <> · <mark>marked</mark>: where their CV has {terms.join(", ")}</>}
          {sortBy !== "newest" && <> · <button type="button" className="link small" onClick={() => { sortList("newest"); }}>Back to newest first</button></>}
        </span>
        {canPick && !emailing && (
          <PickBar count={pick.picked.size} onEmail={canEmail ? () => { setEmailing(true); } : undefined} onClear={pick.clear}
            shown={{ count: hits.length, more: shownList.hasNextPage, all: allShown, onAll: () => { pick.pick(hits.map((h) => personOf(h.candidate)), true); } }}>
            {folders.actions([...pick.ids])}
          </PickBar>
        )}
      </div>
      {emailing && <SeparateEmail people={pick.people} onClose={() => { setEmailing(false); pick.clear(); }} />}
      {openAs === "resume" && (
        <ResumeReader key={JSON.stringify([q, mine, adv, sortBy, folder])} hits={hits} highlight={advanced ? effective.boolean.trim() : ""} seen={seen} onSeen={markSeen}
          chosen={canPick ? pick.ids : undefined} onChoose={canPick ? (c, on) => { pick.pick([personOf(c)], on); } : undefined} chooseToEmail={canEmail}
          canAddToJob={canAddToJob} more={more} onMore={shownList.hasNextPage ? () => { void shownList.fetchNextPage(); } : undefined} />
      )}
      {openAs === "profile" && widths.set && (
        <div className="widths-reset"><button type="button" className="link small" onClick={widths.reset}>Reset column widths</button></div>
      )}
      {openAs === "profile" && <PinnedScroll>
        <table ref={tableRef} {...picking.tableProps} className={`candidates-table${canPick ? " pick-table" : ""}${widths.set ? " sized" : ""}`} style={widths.tableStyle}>
          <thead>
            <tr>
              <SortHeader column="name" label="Name" sort={sortBy} onSort={sortList} canSeeSalary={canSeeSalary} widths={widths} />
              {cols.columns.map((col) => <SortHeader key={col} column={col} label={COLUMNS[col].header} sort={sortBy} onSort={sortList} canSeeSalary={canSeeSalary} widths={widths} move={moveColumn} />)}
              {folders.hasFolders && <th {...widths.head("folders", "In folders")}>In folders{widths.handle("folders", "In folders")}</th>}
            </tr>
          </thead>
          <tbody>
            {hits.map(({ candidate: c, matchedIn, cvSnippet }) => (
              <Fragment key={c.id}>
                <tr {...rowProps(c, cvSnippet)}>
                  <td className="name-cell" {...folders.drag(c.id, pick.ids)}>
                    <WaName c={c} href={`#/candidates/${c.id}${hl}`}>
                      {" "}
                      {!cols.columns.includes("tags") && c.tags.length > 0 && <Chips labels={c.tags} />}
                      {matchedIn.length > 0 && <span className="found-in">{matchedIn.includes("cv") ? "in CV" : "in profile"}</span>}
                      {canPick && pick.ids.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
                    </WaName>
                  </td>
                  {cols.columns.map((col) => <td key={col} className={`col-${col}`}>{COLUMNS[col].cell(c, cols.look)}</td>)}
                  {folders.hasFolders && <td className="col-folders"><InFolders names={inFolders(c.id)} /></td>}
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
      </PinnedScroll>}
      {openAs === "profile" && more}
      </WithFolders>
    </main>
  );
}

type Hit = Schemas["CandidateSearchHit"];

// The chosen order is kept on this device (a convenience only).
const SORT_KEY = "hiringplug.candidates.sort";
function readSort(): CandidateSort {
  try { return (window.localStorage.getItem(SORT_KEY) ?? "newest") as CandidateSort; } catch { return "newest"; }
}
function keepSort(v: CandidateSort) {
  try { window.localStorage.setItem(SORT_KEY, v); } catch { /* only a convenience */ }
}

/** A column header; for sortable columns, a button that sorts (and flips) the list, like a spreadsheet. */
function SortHeader({ column, label, sort, onSort, canSeeSalary, widths, move }: {
  column: string; label: string; sort: CandidateSort; onSort: (s: CandidateSort) => void; canSeeSalary: boolean; widths: ColumnWidths;
  /** Given, the heading can be dragged to move the column. */
  move?: ColumnMove<Column>;
}) {
  const sortable = SORTABLE[column];
  const edge = widths.handle(column, label);
  const moving = move?.props(column as Column); // only given for the chosen columns
  if (!sortable || (sortable.salary && !canSeeSalary)) return <th {...widths.head(column, label)} {...moving}>{label}{edge}</th>;
  const way = sortOf(sort, column);
  return (
    <th {...widths.head(column, label)} {...moving} aria-sort={way ?? "none"}>
      <button type="button" className={`sort-header${way ? " on" : ""}`} onClick={() => { onSort(flipSort(sort, column)); }}>
        {label}<span aria-hidden="true" className="sort-arrow">{way === "ascending" ? "▲" : way === "descending" ? "▼" : "↕"}</span>
      </button>
      {edge}
    </th>
  );
}

const FunnelIcon = () => (
  <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round"><path d="M2 3h12l-4.6 5.6v4.2l-2.8 1.4V8.6z" /></svg>
);
