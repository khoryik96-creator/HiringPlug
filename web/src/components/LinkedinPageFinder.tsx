// A company's LinkedIn page, pasted or found from its name by the LinkedIn
// companion (its companySearch reads LinkedIn's company search in the
// recruiter's own browser). Picking one of the companies found fills the box.
import { useMutation } from "@tanstack/react-query";
import { useEffect, useId, useRef } from "react";

import { companionAtLeast, companionCall, operationId, useCompanion } from "../companion";
import { ErrorMessage } from "./ErrorMessage";
import { companyPage } from "./linkedinCompany";

/** The companion version that finds a company's page from its name. */
export const SEARCH_NEEDS = "1.6.0";

export interface LinkedinCompanyFound { url: string; name: string; subtitle: string }

export function LinkedinPageFinder({ name, value, onChange, disabled = false }: {
  name: string; value: string; onChange: (url: string) => void; disabled?: boolean;
}) {
  const companion = useCompanion();
  const id = useId();
  const canSearch = companion.ready === true && companionAtLeast(companion.version, SEARCH_NEEDS);
  // The search running in the companion: stopped when the box goes away or
  // another starts, or the companion stays busy until it ends on its own.
  const running = useRef<string | null>(null);
  const stop = () => {
    const op = running.current;
    running.current = null;
    if (op) void companionCall("cancel", { operation_id: op }, 5_000).catch(() => undefined);
  };
  useEffect(() => stop, []);
  const search = useMutation({
    mutationFn: async () => {
      stop();
      const op = operationId();
      running.current = op;
      try {
        return (await companionCall<{ companies: LinkedinCompanyFound[] }>("companySearch", { operation_id: op, name: name.trim().slice(0, 200) })).companies;
      } finally {
        if (running.current === op) running.current = null;
      }
    },
  });
  const found = search.data;
  const wrong = value.trim() !== "" && companyPage(value) === "";
  return (
    <div className="linkedin-finder">
      <label className="finder-label" htmlFor={id}>LinkedIn page</label>
      <div className="row tight">
        <input id={id} className="grow" placeholder="linkedin.com/company/…" value={value} disabled={disabled} maxLength={300}
          aria-invalid={wrong || undefined} onChange={(e) => { onChange(e.target.value); }} />
        {canSearch && <button type="button" className="doc-btn" disabled={disabled || search.isPending || !name.trim()}
          onClick={() => { search.mutate(); }}>{search.isPending ? "Finding…" : "Find on LinkedIn"}</button>}
      </div>
      {wrong && <p className="warning small">That is not a LinkedIn company page (linkedin.com/company/…).</p>}
      {companion.ready === false && <p className="muted small">Paste its LinkedIn page. With the LinkedIn companion installed, HiringPlug finds it from the name.</p>}
      {companion.ready === true && !canSearch && <p className="muted small">Paste its LinkedIn page, or update the LinkedIn companion to {SEARCH_NEEDS} to find it from the name.</p>}
      <ErrorMessage error={search.error} />
      {found?.length === 0 && <p className="muted small">LinkedIn found no company by this name. Paste its page instead.</p>}
      {found && found.length > 0 && (
        <ul className="linkedin-found" aria-label="Found on LinkedIn">
          {found.map((c) => {
            const chosen = companyPage(value) === c.url;
            return (
              <li key={c.url}>
                <button type="button" aria-pressed={chosen} disabled={disabled} onClick={() => { onChange(c.url); }}>
                  <span className="linkedin-found-name">{c.name}</span>
                  {c.subtitle && <span className="muted small">{c.subtitle}</span>}
                </button>
                <a className="muted small" href={c.url} target="_blank" rel="noreferrer" aria-label={`Open ${c.name} on LinkedIn`}>↗</a>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
