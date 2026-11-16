import { useInfiniteQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { companiesKey } from "../api/keys";
import { CompanyForm } from "../components/CompanyForm";
import { chipStyle, UNKNOWN_CATEGORY } from "../components/companyCategory";
import { ErrorMessage } from "../components/ErrorMessage";
import { InFolders, useFolderView, useInFolders, WithFolders } from "../components/folders/FolderView";
import { pickRowProps } from "../components/pickRow";
import { PickBar } from "../components/SeparateEmail";
import { useFieldSettings } from "../components/useFieldSettings";
import { COMPANY_TYPE, keysOf } from "../labels";
import { hashParam } from "../hashQuery";
import { PinnedScroll } from "../components/StickyScrollbar";
import { useColumnWidths } from "../components/useColumnWidths";

type TypeFilter = "" | Schemas["CompanyType"];

/** Search, filter and add companies. */
export function CompanyList({ canCreate, canFolders = false, meId = "" }: { canCreate: boolean; canFolders?: boolean; meId?: string }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [type, setType] = useState<TypeFilter>("");
  const [category, setCategory] = useState("");
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
  const list = useInfiniteQuery({
    queryKey: [...companiesKey, q, type, category, folder],
    enabled: folders.ready,
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }) => {
      const query: { q?: string; type?: Schemas["CompanyType"]; category?: string; folder?: string; limit: number; cursor?: string } = { limit: 100 };
      if (pageParam) query.cursor = pageParam;
      if (folder) query.folder = folder;
      if (q.trim()) query.q = q.trim();
      if (type) query.type = type;
      if (category) query.category = category;
      return unwrap(await api.GET("/v1/companies", { params: { query } }));
    },
    getNextPageParam: (last) => last.nextCursor,
  });
  const items = list.data?.pages.flatMap((page) => page.items);
  const inFolders = useInFolders(folders, items?.map((c) => c.id) ?? []);
  const heads: [string, string][] = [
    ["name", "Name"], ["category", "Category"], ["type", "Type"], ["industry", "Industry"], ["city", "City"], ["website", "Website"], ["linkedin", "LinkedIn"],
    ...(folders.hasFolders ? [["folders", "In folders"] as [string, string]] : []),
  ];
  const tableRef = useRef<HTMLTableElement>(null);
  const widths = useColumnWidths("companies", heads.map(([k]) => k), tableRef);

  return (
    <main className="list-page">
      <div className="row spread">
        <h2>Companies</h2>
        <div className="row">
          <input aria-label="Search companies" placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <select aria-label="Type" value={type} onChange={(e) => { setType(e.target.value as TypeFilter); }}>
            <option value="">All types</option>
            {keysOf(COMPANY_TYPE).map((t) => <option key={t} value={t}>{COMPANY_TYPE[t]}</option>)}
          </select>
          <select aria-label="Category" value={category} onChange={(e) => { setCategory(e.target.value); }}>
            <option value="">All categories</option>
            {fs.options("company_category").filter((o) => o.key !== UNKNOWN_CATEGORY).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
          </select>
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add company</button>}
        </div>
      </div>
      {adding && (
        <CompanyForm
          company={null}
          onDone={() => { setAdding(false); }}
          onSaved={(c) => { window.location.hash = `#/companies/${c.id}`; }}
        />
      )}
      <ErrorMessage error={list.error} />
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
      {widths.set && <div className="widths-reset"><button type="button" className="link small" onClick={widths.reset}>Reset column widths</button></div>}
      <PinnedScroll>
      <table ref={tableRef} className={[canFolders ? "pick-table" : "", widths.set ? "sized" : ""].filter(Boolean).join(" ") || undefined} style={widths.tableStyle}>
        <thead>
          <tr>
            {heads.map(([k, label]) => <th key={k} data-column={k} style={widths.style(k)}>{label}{widths.handle(k, label)}</th>)}
          </tr>
        </thead>
        <tbody>
          {items?.map((c) => (
            <tr key={c.id} {...(canFolders ? pickRowProps(chosen.has(c.id), () => { choose([c.id], !chosen.has(c.id)); }) : {})}>
              <td {...folders.drag(c.id, chosen)}>
                <a href={`#/companies/${c.id}`}>{c.name}</a>
                {chosen.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
              </td>
              <td>
                <span className="category-chip" style={chipStyle(fs.color("company_category", c.category ?? UNKNOWN_CATEGORY))}>
                  {fs.label("company_category", c.category ?? UNKNOWN_CATEGORY)}
                </span>
              </td>
              <td>{COMPANY_TYPE[c.type]}</td>
              <td>{c.industry}</td>
              <td>{c.city}</td>
              <td>{c.website && <a href={c.website} target="_blank" rel="noreferrer">{c.website.replace(/^https?:\/\//, "")}</a>}</td>
              <td>{c.linkedinUrl && <a href={c.linkedinUrl} target="_blank" rel="noreferrer" aria-label={`${c.name} on LinkedIn`}>{c.linkedinUrl.replace(/^https:\/\/www\.linkedin\.com\/(company\/)?/, "")}</a>}</td>
              {folders.hasFolders && <td className="col-folders"><InFolders names={inFolders(c.id)} /></td>}
            </tr>
          ))}
          {items?.length === 0 && (
            <tr><td colSpan={folders.hasFolders ? 8 : 7} className="muted">
              {q || type || category ? "No companies match." : folder ? "Nothing in this folder yet: choose companies under All companies, then Add to folder (or drag them onto it)." : "No companies yet."}
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
