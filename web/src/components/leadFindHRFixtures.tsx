// What the lead Find HR panel's tests share: a lead, its company, the companies named like it.
import { vi } from "vitest";

import { api, type Company, type Permission, type Schemas } from "../api/client";
import * as companion from "../companion";
import { ok, renderWithClient } from "../test-utils";
import { LeadFindHR } from "./LeadFindHR";

export const lead: Schemas["JobLead"] = {
  id: "lead_1", version: 4, sourceId: null, sourceKind: null, companyName: "Contoh Logistik Sdn Bhd", companyId: null,
  title: "Warehouse Supervisor", location: "Shah Alam, MY", url: null, postedAt: null,
  firstSeenAt: "2026-11-01T00:00:00Z", lastSeenAt: "2026-11-03T00:00:00Z", closedAt: null, status: "new", notes: null,
};
export const company = {
  id: "comp_1", version: 2, name: "Contoh Logistik Sdn Bhd", city: "Shah Alam", linkedinUrl: "https://www.linkedin.com/company/contoh-logistik",
} as Company;
export const named: Schemas["CompanyNamedLike"][] = [
  { id: "comp_1", name: "Contoh Logistik Sdn Bhd", city: "Shah Alam", website: null, linkedinUrl: company.linkedinUrl, industry: null, sameName: true },
  { id: "comp_2", name: "Contoh Logistics Berhad", city: null, website: null, linkedinUrl: null, industry: null, sameName: false },
];
export const ALL: Permission[] = ["leads:read", "leads:manage", "companies:read", "companies:create", "companies:update", "contacts:read", "contacts:create"];

export function serve({ matches = named, saved = company, version = "1.6.0" }: { matches?: Schemas["CompanyNamedLike"][]; saved?: Company; version?: string } = {}) {
  vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: true, version });
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/companies/named-like" ? { items: matches }
      : path === "/v1/companies/{companyId}/hr-scan" ? { scan: null }
        : path === "/v1/companies/{companyId}" ? saved : { items: [] },
  ))) as never);
}

export const show = (over: Partial<Schemas["JobLead"]> = {}, permissions = ALL, onClose = vi.fn()) =>
  renderWithClient(<LeadFindHR lead={{ ...lead, ...over }} permissions={permissions} onClose={onClose} />);
