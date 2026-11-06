import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { companiesKey, companyAutofillKey, companyKey } from "../api/keys";
import { COMPANY_SIZE, SUGGESTION_FIELD, countryName, formatWhen } from "../labels";
import { ErrorMessage } from "./ErrorMessage";

type Suggestion = Schemas["CompanySuggestion"];

/** A suggested or recorded value in words (sizes and countries are codes). */
export function shownValue(field: Suggestion["field"], value: string): string {
  if (field === "size" && value in COMPANY_SIZE) return COMPANY_SIZE[value as Schemas["CompanySize"]];
  if (field === "country") return countryName(value);
  return value;
}

/**
 * Details read from the company's own website, as suggestions to accept or
 * dismiss one by one. Shown to those who may change the company.
 */
export function CompanyAutofill({ companyId, website }: { companyId: string; website: string | null }) {
  const queryClient = useQueryClient();
  const state = useQuery({
    queryKey: companyAutofillKey(companyId),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/autofill", { params: { path: { companyId } } })),
    refetchInterval: (q) => (q.state.data?.status === "queued" ? 2000 : false),
    enabled: website !== null,
  });
  const show = (data: Schemas["CompanyAutofill"]) => { queryClient.setQueryData(companyAutofillKey(companyId), data); };
  const start = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/companies/{companyId}/autofill", { params: { path: { companyId } } })),
    onSuccess: show,
  });
  const decide = useMutation({
    mutationFn: async ({ id, accept }: { id: string; accept: boolean }) => unwrap(await api.POST("/v1/companies/{companyId}/autofill/{suggestionId}", {
      params: { path: { companyId, suggestionId: id } }, body: { accept },
    })),
    onSuccess: async (data, { accept }) => {
      show(data);
      if (accept) {
        await queryClient.invalidateQueries({ queryKey: companyKey(companyId), exact: true });
        await queryClient.invalidateQueries({ queryKey: companiesKey });
      }
    },
  });

  if (!website) return null;
  const a = state.data;
  const busy = a?.status === "queued" || start.isPending;
  return (
    <section className="panel" aria-label="From the website">
      <div className="row spread">
        <h3>From the website</h3>
        <button type="button" disabled={busy || !a} onClick={() => { start.mutate(); }}>
          {a?.checkedAt ? "Read the website again" : "Fill in from website"}
        </button>
      </div>
      <ErrorMessage error={state.error ?? start.error ?? decide.error} />
      {busy && <p className="muted" role="status">Reading the website…</p>}
      {a?.status === "failed" && a.error && <p className="warning" role="status">{a.error}</p>}
      {a?.status === "done" && a.suggestions.length === 0 && (
        <p className="muted">Nothing new on the website{a.checkedAt && <> (read {formatWhen(a.checkedAt)})</>}.</p>
      )}
      {a && a.suggestions.length > 0 && (
        <ul className="suggestions">
          {a.suggestions.map((s) => (
            <li key={s.id} aria-label={SUGGESTION_FIELD[s.field]}>
              <strong>{SUGGESTION_FIELD[s.field]}:</strong> <span className={s.field === "about" ? "pre" : undefined}>{shownValue(s.field, s.value)}</span>
              <div className="muted">
                {s.current ? <>Now: {shownValue(s.field, s.current)} · </> : null}From {s.source}
              </div>
              <div className="row">
                <button type="button" className="link" disabled={decide.isPending} onClick={() => { decide.mutate({ id: s.id, accept: true }); }}>
                  {s.current ? "Replace" : "Accept"}
                </button>
                <button type="button" className="link" disabled={decide.isPending} onClick={() => { decide.mutate({ id: s.id, accept: false }); }}>Dismiss</button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
