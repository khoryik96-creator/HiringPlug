// Settings > System: the company's background jobs (what is waiting, what
// failed for good, with Try again) and the server's last backup.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { formatWhen } from "../labels";

const systemKey = ["settings", "system"] as const;

/** What each background job does, in a recruiter's words (a new kind in the contract must be named here). */
export const JOB_LABELS: Record<Schemas["JobKind"], string> = {
  "aiprovider.check": "Checking an AI key",
  "aiprovider.checked": "Saving an AI key check",
  "assistant.ask": "AI assistant questions",
  "assistant.answered": "Saving AI assistant answers",
  "automation.event": "Starting automations",
  "automation.run": "Running automations (emails and tasks)",
  "company.autofill": "Filling in companies from their websites",
  "contact.read_linkedin": "Reading contacts' LinkedIn profiles",
  "contact.check_mailbox": "Verifying contacts' email addresses",
  "email.bounce_scan": "Looking for bounced emails in Outlook",
  "contact.linkedin_read": "Saving contacts' LinkedIn profiles",
  "cv.format_studio": "Formatting CVs CV Studio's way",
  "cv.studio_formatted": "Saving CV Studio formatted CVs",
  "document.extract_text": "Reading CVs",
  "document.text_extracted": "Saving CV text",
  "document.parse_cv": "Reading details from CVs",
  "document.cv_parsed": "Saving details read from CVs",
  "document.render_pdf": "Making PDF copies of Word files",
  "document.pdf_rendered": "Saving PDF copies",
  "email.send": "Sending emails",
  "interview.remind": "Interview reminders",
  "placement.invoice": "Invoices emailed on start dates",
  "jd.blind": "Rewriting blind ads with the AI",
  "jd.blinded": "Saving rewritten blind ads",
  "job.read_jd": "Reading job descriptions",
  "job.jd_read": "Saving job descriptions",
  "lead_source.check": "Checking job boards for leads",
  "match.score": "CV vs job fit scores",
  "match.scored": "Saving fit scores",
  "talent.rank": "AI candidate search: ranking the candidates",
  "talent.ranked": "AI candidate search: saving the ranking",
  "scheduling.refresh_calendar": "Reading busy times from calendars",
  "webhook.deliver": "Sending webhooks to your other systems",
};
const label = (kind: string) => (JOB_LABELS as Record<string, string | undefined>)[kind] ?? kind;

export function System() {
  const queryClient = useQueryClient();
  const status = useQuery({
    queryKey: systemKey,
    queryFn: async () => unwrap(await api.GET("/v1/settings/system")),
    refetchInterval: 10_000,
  });
  const retry = useMutation({
    mutationFn: async (jobId: string) => unwrap(await api.POST("/v1/settings/system/jobs/{jobId}/retry", { params: { path: { jobId } } })),
    onSuccess: (st) => { queryClient.setQueryData(systemKey, st); },
  });
  const st = status.data;
  return (
    <main className="system">
      <h2>System</h2>
      <ErrorMessage error={status.error ?? retry.error} />
      {st && (
        <>
          <Backup backup={st.backup} auto={st.backupAuto} />
          <section className="panel" aria-label="Background jobs">
            <h3>Background jobs</h3>
            {st.jobs.length === 0 ? <p className="muted">Nothing waiting, nothing failed.</p> : (
              <table>
                <thead><tr><th>Work</th><th>Waiting</th><th>Running</th><th>Retrying</th><th>Failed (30 days)</th><th>Waiting since</th></tr></thead>
                <tbody>
                  {st.jobs.map((j) => (
                    <tr key={j.kind}>
                      <td>{label(j.kind)}</td><td>{j.waiting}</td><td>{j.running}</td><td>{j.retrying}</td>
                      <td>{j.failed > 0 ? <span className="badge failed">{j.failed}</span> : 0}</td>
                      <td className="muted">{j.oldestWaitingSince ? formatWhen(j.oldestWaitingSince) : ""}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
          {st.failed.length > 0 && (
            <section className="panel" aria-label="Failed jobs">
              <h3>Failed</h3>
              <table>
                <thead><tr><th>Work</th><th>Failed</th><th>Why</th><th /></tr></thead>
                <tbody>
                  {st.failed.map((f) => (
                    <tr key={f.id}>
                      <td>{label(f.kind)}</td>
                      <td className="muted">{formatWhen(f.failedAt)} after {f.attempts} tries</td>
                      <td className="small">{f.error}</td>
                      <td><button type="button" className="link" disabled={retry.isPending} onClick={() => { retry.mutate(f.id); }}>Try again</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          )}
        </>
      )}
    </main>
  );
}

function Backup({ backup, auto }: { backup: Schemas["SystemStatus"]["backup"]; auto: boolean }) {
  const [now] = useState(Date.now); // when the page was opened
  // Old only matters while a backup is expected every night.
  const stale = auto && backup?.status === "done" && now - new Date(backup.startedAt).getTime() > 2 * 24 * 3600 * 1000;
  return (
    <section className="panel" aria-label="Backups">
      <h3>Backups</h3>
      {!auto && <p className="muted">The nightly backup is turned off (Settings &gt; Backups): backups are made only when asked for.</p>}
      {!backup && auto && <p className="warning">No backup has been made yet. With Docker one is made every night; see the guide to run them.</p>}
      {backup?.status === "done" && (
        <p className={stale ? "warning" : undefined}>
          Last backup {formatWhen(backup.startedAt)}{backup.sizeBytes !== null && ` (${megabytes(backup.sizeBytes)})`}: {backup.name}.
          {stale && " That is more than two days ago: check that the backup service is running."}
        </p>
      )}
      {backup?.status === "running" && <p>A backup is being made (started {formatWhen(backup.startedAt)}).</p>}
      {backup?.status === "failed" && (
        <p className="warning" role="alert">The last backup failed ({formatWhen(backup.startedAt)}): {backup.error}</p>
      )}
    </section>
  );
}

export const megabytes = (n: number) => (n < 1024 * 1024 ? `${Math.max(1, Math.round(n / 1024))} KB` : `${(n / 1024 / 1024).toFixed(1)} MB`);
