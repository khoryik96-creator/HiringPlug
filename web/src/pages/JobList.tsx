import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { jobsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { JobForm } from "../components/JobForm";
import { JobTable } from "../components/JobTable";
import { JOB_STATUS, keysOf } from "../labels";

type StatusFilter = "" | Schemas["JobStatus"];

/** Search, filter and add jobs. Shows open jobs first by default. */
export function JobList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState("");
  const [status, setStatus] = useState<StatusFilter>("open");
  const [adding, setAdding] = useState(false);
  const list = useQuery({
    queryKey: [...jobsKey, q, status],
    queryFn: async () => {
      const query: { q?: string; status?: Schemas["JobStatus"] } = {};
      if (q.trim()) query.q = q.trim();
      if (status) query.status = status;
      return unwrap(await api.GET("/v1/jobs", { params: { query } }));
    },
  });

  return (
    <main>
      <div className="row spread">
        <h2>Jobs</h2>
        <div className="row">
          <input aria-label="Search jobs" placeholder="Search title or location" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <select aria-label="Status" value={status} onChange={(e) => { setStatus(e.target.value as StatusFilter); }}>
            <option value="">All statuses</option>
            {keysOf(JOB_STATUS).map((s) => <option key={s} value={s}>{JOB_STATUS[s]}</option>)}
          </select>
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add job</button>}
        </div>
      </div>
      {adding && (
        <JobForm job={null} onDone={() => { setAdding(false); }} onSaved={(j) => { window.location.hash = `#/jobs/${j.id}`; }} />
      )}
      <ErrorMessage error={list.error} />
      {list.data && <JobTable jobs={list.data.items} showCompany empty={q || status ? "No jobs match." : "No jobs yet."} />}
    </main>
  );
}
