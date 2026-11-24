// Leads › Find HR: a linked company without a LinkedIn company page in
// HiringPlug. Its page is pasted or found, checked against the companies that
// have it already, and saved; a link that is not a company page (lnkd.in, a
// showcase or school page) is shown and replaced only once the user says so.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Company, unwrap } from "../api/client";
import { companyKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { companyPage, companySlug } from "./linkedinCompany";
import { LinkedinPageFinder } from "./LinkedinPageFinder";

export function CompanyLinkedin({ company, canUpdate }: { company: Company; canUpdate: boolean }) {
  const queryClient = useQueryClient();
  const [url, setUrl] = useState("");
  // A company in HiringPlug has the page chosen already: saving it here would make a second.
  const [pageOwner, setPageOwner] = useState<{ page: string; id: string; name: string } | null>(null);
  // The page that would replace the company's link, waiting for a yes.
  const [replacing, setReplacing] = useState<string | null>(null);
  const current = company.linkedinUrl?.trim() ?? "";
  const save = useMutation({
    mutationFn: async (confirmed: boolean): Promise<Company | null> => {
      const page = companyPage(url);
      if (!confirmed) {
        const slug = companySlug(page);
        const owner = slug ? unwrap(await api.GET("/v1/companies", { params: { query: { linkedinCompany: slug, limit: 1 } } })).items[0] : undefined;
        if (owner && owner.id !== company.id) {
          setPageOwner({ page, id: owner.id, name: owner.name });
          return null;
        }
        if (current) {
          setReplacing(page);
          return null;
        }
      }
      return unwrap(await api.PATCH("/v1/companies/{companyId}", {
        params: { path: { companyId: company.id } }, body: { version: company.version, linkedinUrl: page },
      }));
    },
    onSuccess: (saved) => {
      if (!saved) return;
      setReplacing(null);
      queryClient.setQueryData(companyKey(company.id), saved);
      void queryClient.invalidateQueries({ queryKey: ["companies"] });
    },
    // Changed by someone else, say: read again, so saving again sends its current version.
    onError: () => { void queryClient.invalidateQueries({ queryKey: companyKey(company.id) }); },
  });
  const page = companyPage(url);
  const samePage = pageOwner && pageOwner.page === page ? pageOwner : null;
  const confirming = replacing !== null && replacing === page;
  const notAPage = current && <p className="warning small">Its link in HiringPlug, <code>{current}</code>, is not a LinkedIn company page.</p>;
  if (!canUpdate) {
    return current
      ? <>{notAPage}<p className="muted small">Changing it needs the permission to edit companies.</p></>
      : <p className="muted small">{company.name} has no LinkedIn page in HiringPlug yet, and adding it needs the permission to edit companies.</p>;
  }
  return (
    <form className="lead-hr-step" aria-label="Its LinkedIn page" onSubmit={(e) => { e.preventDefault(); save.mutate(false); }}>
      <h4>Its LinkedIn page</h4>
      {notAPage}
      <LinkedinPageFinder name={company.name} value={url} onChange={setUrl} disabled={save.isPending} />
      {samePage && (
        <div className="warning small" role="status">
          <p>{samePage.name} in HiringPlug has this LinkedIn page. Choose another page, or merge the two companies.</p>
          <a href={`#/companies/${samePage.id}`} target="_blank" rel="noreferrer">Open {samePage.name} ↗</a>
        </div>
      )}
      {confirming && (
        <div className="warning small" role="status">
          <p>Replace {current} with {page}?</p>
          <button type="button" className="doc-btn" disabled={save.isPending} onClick={() => { save.mutate(true); }}>Replace it</button>
        </div>
      )}
      <ErrorMessage error={save.error} />
      <div className="row">
        <button type="submit" className="primary" disabled={save.isPending || !page || samePage !== null || confirming}>{save.isPending ? "Saving…" : "Save page"}</button>
      </div>
    </form>
  );
}
