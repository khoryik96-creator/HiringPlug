// Helpers for component tests: a fresh query client per test, sample data, and
// the result shapes the API client (openapi-fetch) resolves to, for mocks.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";

import type { Candidate, ErrorBody, Me, Permission } from "./api/client";

export function testCandidate(overrides: Partial<Candidate> = {}): Candidate {
  return {
    id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 3, firstName: "Siti", lastName: "Aisyah", fullName: "Siti Aisyah",
    email: "siti@example.com", phone: null, nationality: "MY", workAuthorization: null,
    noticePeriod: { value: 1, unit: "month" }, industry: null, specialization: null, currentTitle: "Accountant",
    currentCompany: null, location: null, source: "jobstreet", restrictedFields: [],
    createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
    ...overrides,
  };
}

export function testMe(permissions: Permission[]): Me {
  return {
    user: { id: "usr_01jbz6w0m0k8q9x2r3s4t5v6w7", email: "demo@example.com", name: "Demo" },
    tenant: { id: "ten_01jbz6w0m0k8q9x2r3s4t5v6w7", name: "Demo company" },
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
