import { useInfiniteQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { StaleFilter, StaleTag } from "../components/StaleTag";

import { api, type Schemas, unwrap } from "../api/client";
import { companiesKey } from "../api/keys";
import { ExportButton } from "../components/ExportButton";
import { downloadExport } from "../api/files";
import { IconButton, PlusIcon } from "../components/ActionIcons";
import { CompanyForm } from "../components/CompanyForm";
import { chipStyle, UNKNOWN_CATEGORY } from "../components/companyCategory";
import { useDebounced } from "../components/useDebounced";
import { BooleanSearchBox, ListSearchError, looksBoolean } from "../components/search/BooleanSearchBox";
import { InFolders, useFolderView, useInFolders, WithFolders } from "../components/folders/FolderView";
import { useRowPicking } from "../components/useRowPicking";
import { PickBar } from "../components/SeparateEmail";
import { useFieldSettings } from "../components/useFieldSettings";
import { COMPANY_TYPE, keysOf } from "../labels";
import { hashParam } from "../hashQuery";
import { PinnedScroll } from "../components/StickyScrollbar";
import { useColumnMove, useStoredOrder } from "../components/useColumnMove";
import { useColumnWidths } from "../components/useColumnWidths";
import { CountryTag } from "../components/CountryTag";

type TypeFilter = "" | Schemas["CompanyType"];

/** Search, filter and add companies. */
type Company = Schemas["CompanySummary"];
type FieldSettings = ReturnType<typeof useFieldSettings>;

/** The columns after the name, in the order they come: their heading and cell. */
const CELLS = {
  category: { label: "Category", cell: (c: Company, fs: FieldSettings) => (
    <span className="category-chip" style={chipStyle(fs.color("company_category", c.category ?? UNKNOWN_CATEGORY))}>
      {fs.label("company_category", c.category ?? UNKNOWN_CATEGORY)}
    </span>
  ) },
  type: { label: "Type", cell: (c: Company) => COMPANY_TYPE[c.type] },
  industry: { label: "Industry", cell: (c: Company) => c.industry },
  city: { label: "City", cell: (c: Company) => c.city },
  website: { label: "Website", cell: (c: Company) => c.website && <a href={c.website} target="_blank" rel="noreferrer">{c.website.replace(/^https?:\/\//, "")}</a> },
  linkedin: { label: "LinkedIn", cell: (c: Company) => c.linkedinUrl && (
    <a href={c.linkedinUrl} target="_blank" rel="noreferrer" aria-label={`${c.name} on LinkedIn`}>{c.linkedinUrl.replace(/^https:\/\/www\.linkedin\.com\/(company\/)?/, "")}</a>
  ) },
};

export function CompanyList({ canCreate, canExport = false, canFolders = false, meId = "" }: { canCreate: boolean; canExport?: boolean; canFolders?: boolean; meId?: string }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [type, setType] = useState<TypeFilter>("");
  const [category, setCategory] = useState("");
  const [stale, setStale] = useState(() => hashParam("stale") === "1"); // from the morning email's link
  const fs = useFieldSettings();
  const [adding, setAdding] = useState(false);
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set()); // to put in a folder
  const folders = useFolderView("company", canFolders, meId, () => { setChosen(new Set()); });
  const folder = folders.filter;
  const choose = (ids: string[], on: boolean) => {
    setChosen((cur) => {
      const next = new Set(cur);
      for (const id of ids) { if (on) next.add(id); else next.delete(id); }
      return next;
    });
  };
  const typed = useDebounced(q, 350); // one search once typing pauses
  const list = useInfiniteQuery({
    queryKey: [...companiesKey, typed, type, category, stale, folder],
    enabled: folders.ready,
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }) => {
      const query: { q?: string; boolean?: string; type?: Schemas["CompanyType"]; category?: string; stale?: boolean; folder?: string; limit: number; cursor?: string } = { limit: 100 };
      if (pageParam) query.cursor = pageParam;
      if (folder) query.folder = folder;
      if (typed.trim()) query[looksBoolean(typed) ? "boolean" : "q"] = typed.trim(); // AND, OR, NOT…: a boolean search
      if (type) query.type = type;
      if (category) query.category = category;
      if (stale) query.stale = true;
      return unwrap(await api.GET("/v1/companies", { params: { query } }));
    },
    getNextPageParam: (last) => last.nextCursor,
  });
  const items = list.data?.pages.flatMap((page) => page.items);
  const picking = useRowPicking({ ids: items?.map((c) => c.id) ?? [], chosen, set: choose, clear: () => { setChosen(new Set()); }, enabled: canFolders });
  const inFolders = useInFolders(folders, items?.map((c) => c.id) ?? []);
  // After the name, the columns are in the order the person dragged them into.
  const columns = useStoredOrder("companies", Object.keys(CELLS) as (keyof typeof CELLS)[]);
  const tableRef = useRef<HTMLTableElement>(null);
  const moveColumn = useColumnMove(tableRef, columns.order, columns.save);
  const widths = useColumnWidths("companies", ["name", ...columns.order, ...(folders.hasFolders ? ["folders"] : [])], tableRef);


  return (
    <main className="list-page">
      <h2 className="visually-hidden">Companies</h2>
      <div className="list-top">
        <BooleanSearchBox value={q} onChange={setQ} label="Search companies" placeholder="Search by name, website or phone, or AND, OR, NOT…">
          <select aria-label="Type" className={`search-pill${type ? " on" : ""}`} value={type} onChange={(e) => { setType(e.target.value as TypeFilter); }}>
            <option value="">All types</option>
            {keysOf(COMPANY_TYPE).map((t) => <option key={t} value={t}>{COMPANY_TYPE[t]}</option>)}
          </select>
          <select aria-label="Category" className={`search-pill${category ? " on" : ""}`} value={category} onChange={(e) => { setCategory(e.target.value); }}>
            <option value="">All categories</option>
            {fs.options("company_category").filter((o) => o.key !== UNKNOWN_CATEGORY).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
          </select>
          <StaleFilter on={stale} onChange={setStale} />
        </BooleanSearchBox>
        <a className="dupes-link" href="#/companies/duplicates">Possible duplicates</a>
        {canExport && <ExportButton what="companies" run={(format) => {
          const query = new URLSearchParams({ format });
          if (folder) query.set("folder", folder);
          if (typed.trim()) query.set(looksBoolean(typed) ? "boolean" : "q", typed.trim());
          if (type) query.set("type", type);
          if (category) query.set("category", category);
          if (stale) query.set("stale", "true");
          return downloadExport(`/v1/companies/export?${query.toString()}`);
        }} />}
        {canCreate && !adding && <IconButton label="Add company" icon={<PlusIcon />} className="icon-primary" onClick={() => { setAdding(true); }} />}
      </div>
      {adding && (
        <CompanyForm
          company={null}
          onDone={() => { setAdding(false); }}
          onSaved={(c) => { window.location.hash = `#/companies/${c.id}`; }}
        />
      )}
      <ListSearchError error={list.error} />
      <WithFolders view={folders}>
      {canFolders && items && (
        <div className="list-meta">
          <span role="status">{items.length}{list.hasNextPage ? "+" : ""} compan{items.length === 1 ? "y" : "ies"}</span>
          <PickBar count={chosen.size} onClear={() => { setChosen(new Set()); }}
            shown={{ count: items.length, more: list.hasNextPage, all: items.length > 0 && items.every((c) => chosen.has(c.id)), onAll: () => { choose(items.map((c) => c.id), true); } }}>
            {folders.actions([...chosen])}
          </PickBar>
        </div>
      )}
      {(widths.set || columns.changed) && (
        <div className="widths-reset">
          {columns.changed && <button type="button" className="link small" onClick={columns.reset}>Reset column order</button>}
          {widths.set && <button type="button" className="link small" onClick={widths.reset}>Reset column widths</button>}
        </div>
      )}
      <PinnedScroll>
      <table ref={tableRef} {...picking.tableProps} className={[canFolders ? "pick-table" : "", widths.set ? "sized" : ""].filter(Boolean).join(" ") || undefined} style={widths.tableStyle}>
        <thead>
          <tr>
            <th {...widths.head("name", "Name")}>Name{widths.handle("name", "Name")}</th>
            {columns.order.map((k) => {
              const label = CELLS[k].label;
              return <th key={k} {...widths.head(k, label)} {...moveColumn.props(k)}>{label}{widths.handle(k, label)}</th>;
            })}
            {folders.hasFolders && <th {...widths.head("folders", "In folders")}>In folders{widths.handle("folders", "In folders")}</th>}
          </tr>
        </thead>
        <tbody>
          {items?.map((c) => (
            <tr key={c.id} {...picking.rowProps(c.id)}>
              <td {...folders.drag(c.id, chosen)}>
                <a href={`#/companies/${c.id}`}>{c.name}</a> <CountryTag country={c.country} /> <StaleTag touchedAt={c.touchedAt} />
                {chosen.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
              </td>
              {columns.order.map((k) => <td key={k}>{CELLS[k].cell(c, fs)}</td>)}
              {folders.hasFolders && <td className="col-folders"><InFolders names={inFolders(c.id)} /></td>}
            </tr>
          ))}
          {items?.length === 0 && (
            <tr><td colSpan={columns.order.length + (folders.hasFolders ? 2 : 1)} className="muted">
              {q || type || category || stale ? "No companies match." : folder ? "Nothing in this folder yet: choose companies under All companies, then Add to folder (or drag them onto it)." : "No companies yet."}
            </td></tr>
          )}
        </tbody>
      </table>
      </PinnedScroll>
      {list.hasNextPage && (
        <button type="button" className="doc-btn" disabled={list.isFetchingNextPage} onClick={() => { void list.fetchNextPage(); }}>Show more</button>
      )}
      </WithFolders>
    </main>
  );
}
