import type { JobOrderSummary } from "../api/client";
import { JOB_STATUS } from "../labels";

/** Jobs in a table, linking to each job (and its company, if shown). */
export function JobTable({ jobs, showCompany = false, empty }: { jobs: JobOrderSummary[]; showCompany?: boolean; empty: string }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Job</th>
          {showCompany && <th>Company</th>}
          <th>Status</th>
          <th>Location</th>
          <th>Openings</th>
        </tr>
      </thead>
      <tbody>
        {jobs.map((j) => (
          <tr key={j.id}>
            <td><a href={`#/jobs/${j.id}`}>{j.title}</a></td>
            {showCompany && <td><a href={`#/companies/${j.companyId}`}>{j.companyName}</a></td>}
            <td><span className={`badge ${j.status}`}>{JOB_STATUS[j.status]}</span></td>
            <td>{j.location}</td>
            <td>{j.openings}</td>
          </tr>
        ))}
        {jobs.length === 0 && <tr><td colSpan={showCompany ? 5 : 4} className="muted">{empty}</td></tr>}
      </tbody>
    </table>
  );
}
