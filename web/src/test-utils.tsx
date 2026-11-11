// Helpers for component tests: a fresh query client per test, sample data, and
// the result shapes the API client (openapi-fetch) resolves to, for mocks.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";

import type { Candidate, ErrorBody, Me, Permission } from "./api/client";

export function testCandidate(overrides: Partial<Candidate> = {}): Candidate {
  return {
    id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 3, firstName: "Siti", lastName: "Aisyah", fullName: "Siti Aisyah",
    email: "siti@example.com", phone: null, linkedinUrl: null, availability: null, availabilitySince: null, nationality: "MY", residenceCountry: null, workAuthorization: null,
    noticePeriod: { value: 1, unit: "month" }, industry: null, specialization: null, currentTitle: "Accountant",
    currentCompany: null, location: null, source: "jobstreet", tags: [], skills: [], experience: [], education: [], restrictedFields: [],
    ownerId: null, ownerName: null,
    createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
    ...overrides,
  };
}

export function testMe(permissions: Permission[]): Me {
  return {
    user: { id: "usr_01jbz6w0m0k8q9x2r3s4t5v6w7", email: "demo@example.com", name: "Demo" },
    tenant: { id: "ten_01jbz6w0m0k8q9x2r3s4t5v6w7", name: "Demo company", careersSlug: "demo-company" },
    role: "recruiter",
    permissions,
  };
}

export const RECRUITER: Permission[] = [
  "candidates:read", "candidates:create", "candidates:update",
  "candidates.compensation:read", "candidates.compensation:update", "documents:read", "documents:create",
];

/** What an API client call resolves to on success. */
export function ok(data: unknown): never {
  return { data, error: undefined, response: new Response(null, { status: 200 }) } as never;
}

/** What an API client call resolves to for an error response. */
export function failure(
  code: ErrorBody["error"]["code"], message: string, details?: Record<string, unknown>, status = 400,
): never {
  const error: ErrorBody = { error: { code, message, requestId: "req_test", ...(details ? { details } : {}) } };
  return { data: undefined, error, response: new Response(null, { status }) } as never;
}

/** A VALIDATION_FAILED response naming one field. */
export function invalidField(field: string, message: string): never {
  return failure("VALIDATION_FAILED", "Some fields are invalid.", { fields: [{ field, message }] });
}

/** Renders with a fresh query client that, like the app's, does not retry. */
export function renderWithClient(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["industries"], testIndustryCatalogue());
  client.setQueryDefaults(["industries"], { staleTime: Infinity });
  const wrap = (node: ReactElement) => <QueryClientProvider client={client}>{node}</QueryClientProvider>;
  const result = render(wrap(ui));
  return { client, rerender: (next: ReactElement) => { result.rerender(wrap(next)); } };
}

/** The body of a mocked API call (calls[n][1].body), whatever the endpoint. */
export function sentBody(mock: { mock: { calls: unknown[][] } }, call = 0): unknown {
  const init = mock.mock.calls[call]?.[1] as { body?: unknown } | undefined;
  return init?.body;
}

/** The settings a company starts with (GET /v1/settings/fields), optionally with mandatory fields. */
export function testFieldSettings(mandatory: { form: "candidate" | "job"; field: string }[] = []): never {
  const list = (name: string, label: string, kind: "open" | "fixed", pairs: [string, string, string?][], hints = false) =>
    ({ name, label, kind, hints, options: pairs.map(([key, l, hint]) => ({ key, label: l, hint: hint ?? null, hidden: false, builtIn: true })) });
  return {
    lists: [
      list("candidate_source", "Candidate source", "open", [["jobstreet", "JobStreet"], ["linkedin", "LinkedIn"], ["referral", "Referral"], ["careers_page", "Careers page"], ["direct", "Direct"], ["other", "Other"]]),
      list("notice_period", "Notice period", "open", [["0_day", "Immediately"], ["2_week", "2 weeks"], ["1_month", "1 month"], ["3_month", "3 months"]]),
      list("industry", "Industry", "open", [["Financial Services", "Financial Services"], ["FMCG", "FMCG"]]),
      list("work_type", "Work type", "fixed", [["full_time", "Full time"], ["contract", "Contract"]]),
      list("work_arrangement", "Work arrangement", "fixed", [["onsite", "On site"], ["hybrid", "Hybrid"], ["remote", "Remote"]]),
      list("company_benefits", "Company benefits", "open", [
        ["annual_leave", "Annual leave", "If it grows with service or grade, give the detail"], ["dental_optical", "Dental & optical"],
        ["fixed_bonus", "Fixed bonus", "13th, 13.5 or 14th month; any specific clause"],
      ], true),
      list("pre_offer_process", "Pre-offer process", "open", [
        ["reference_check", "Reference check", "How many, and which referees (superior/peers)"], ["health_screening", "Health screening"],
      ], true),
    ],
    mandatory: mandatory.map((m) => ({ ...m, label: m.field, mandatory: true, alwaysRequired: false })),
  } as never;
}

/** Synthetic shared catalogue for form tests. Recognition and CRUD tests mock their own requests. */
export function testIndustryCatalogue() {
  return { version: 1, items: [
    { id: "industry_bank", label: "Financial Services", aliases: [], retired: false, candidateCount: 0, companyCount: 0 },
    { id: "industry_banking", parentId: "industry_bank", label: "FSI – Banking", aliases: ["banking"], retired: false, candidateCount: 0, companyCount: 0 },
    { id: "industry_it", label: "Information Technology & Services", aliases: [], retired: false, candidateCount: 0, companyCount: 0 },
    { id: "industry_erp", parentId: "industry_it", label: "Tech – ERP/CRM/HCM/Application", aliases: ["SAP"], retired: false, candidateCount: 0, companyCount: 0 },
    { id: "industry_education", label: "Education", aliases: [], retired: false, candidateCount: 0, companyCount: 0 },
  ] };
}
