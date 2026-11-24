// Settings › Audit log (option A): two tabs, Changes and Salary views, with
// the same filters across the top (who, what, from, to), newest first, and an
// Excel download of what the filters find. The tab and filters are kept in
// the address, so a link reopens the same view. Salary views need their own
// permission; without it there is only the Changes list.
import { useEffect, useState } from "react";

import { SectionTabs } from "../components/SectionTabs";
import { ChangesTable } from "./auditLog/ChangesTable";
import { FilterBar } from "./auditLog/FilterBar";
import { type Filters, filtersFor, readView, type Tab, viewHash } from "./auditLog/filters";
import { SalaryViewsTable } from "./auditLog/SalaryViewsTable";
import "./auditLog/auditLog.css";

export function AuditLog({ canSeeSalaryViews }: { canSeeSalaryViews: boolean }) {
  const [view, setView] = useState(() => readView(window.location.hash, canSeeSalaryViews));
  useEffect(() => {
    const hash = viewHash(view.tab, view.filters);
    if (window.location.hash !== hash) window.history.replaceState(null, "", hash);
  }, [view]);
  const setFilters = (filters: Filters) => { setView((v) => ({ ...v, filters })); };
  const setTab = (tab: Tab) => { setView((v) => ({ tab, filters: filtersFor(tab, v.filters) })); };

  const changes = (
    <div className="audit-panel">
      <FilterBar tab="changes" filters={view.filters} onChange={setFilters} />
      <ChangesTable filters={view.filters} />
    </div>
  );
  return (
    <main className="audit-log">
      <h2>Audit log</h2>
      <p className="muted">
        Who changed what, and when, newest first. Click an entry to see what changed. Salary amounts are never kept here, only that they
        changed{canSeeSalaryViews ? "; Salary views shows who was shown a candidate's salary" : ""}.
      </p>
      <section className="panel">
        {canSeeSalaryViews ? (
          <SectionTabs name="audit" label="Audit log" active={view.tab} onChange={setTab} sections={[
            { id: "changes", label: "Changes", content: changes },
            { id: "salary", label: "Salary views", content: (
              <div className="audit-panel">
                <FilterBar tab="salary" filters={view.filters} onChange={setFilters} />
                <SalaryViewsTable filters={view.filters} />
              </div>
            ) },
          ]} />
        ) : changes}
      </section>
    </main>
  );
}
