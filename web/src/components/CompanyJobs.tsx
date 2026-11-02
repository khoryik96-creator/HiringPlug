import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Company, unwrap } from "../api/client";
import { jobsKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { JobForm } from "./JobForm";
import { JobTable } from "./JobTable";

/** A company's jobs, with "Add job" already set to this company. */
export function CompanyJobs({ company, canCreate }: { company: Company; canCreate: boolean }) {
  const [adding, setAdding] = useState(false);
  const list = useQuery({
    queryKey: [...jobsKey, "company", company.id],
    queryFn: async () => unwrap(await api.GET("/v1/jobs", { params: { query: { companyId: company.id, limit: 100 } } })).items,
  });
  return (
    <section className="panel" aria-label="Jobs">
      <div className="row spread">
        <h3>Jobs</h3>
        {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add job</button>}
      </div>
      {adding && (
        <JobForm job={null} company={{ id: company.id, name: company.name }} onDone={() => { setAdding(false); }}
          onSaved={(j) => { window.location.hash = `#/jobs/${j.id}`; }} />
      )}
      <ErrorMessage error={list.error} />
      {list.data && <JobTable jobs={list.data} empty="No jobs for this company yet." />}
    </section>
  );
}
