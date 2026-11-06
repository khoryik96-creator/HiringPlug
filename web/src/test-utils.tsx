// Helpers for component tests: a fresh query client per test, sample data, and
// the result shapes the API client (openapi-fetch) resolves to, for mocks.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";

import type { Candidate, ErrorBody, Me, Permission } from "./api/client";

export function testCandidate(overrides: Partial<Candidate> = {}): Candidate {
  return {
    id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 3, firstName: "Siti", lastName: "Aisyah", fullName: "Siti Aisyah",
    email: "siti@example.com", phone: null, linkedinUrl: null, availability: null, nationality: "MY", residenceCountry: null, workAuthorization: null,
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
  const list = (name: string, label: string, kind: "open" | "fixed", pairs: [string, string][]) =>
    ({ name, label, kind, options: pairs.map(([key, l]) => ({ key, label: l, hidden: false, builtIn: true })) });
  return {
    lists: [
      list("candidate_source", "Candidate source", "open", [["jobstreet", "JobStreet"], ["linkedin", "LinkedIn"], ["referral", "Referral"], ["careers_page", "Careers page"], ["direct", "Direct"], ["other", "Other"]]),
      list("notice_period", "Notice period", "open", [["0_day", "Immediately"], ["2_week", "2 weeks"], ["1_month", "1 month"], ["3_month", "3 months"]]),
      list("industry", "Industry", "open", [["Financial Services", "Financial Services"], ["FMCG", "FMCG"]]),
      list("work_type", "Work type", "fixed", [["full_time", "Full time"], ["contract", "Contract"]]),
      list("work_arrangement", "Work arrangement", "fixed", [["onsite", "On site"], ["hybrid", "Hybrid"], ["remote", "Remote"]]),
    ],
    mandatory: mandatory.map((m) => ({ ...m, label: m.field, mandatory: true, alwaysRequired: false })),
  } as never;
}
