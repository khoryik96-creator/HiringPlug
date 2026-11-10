// The typed API client. Every path, parameter and response type comes from
// schema.d.ts, generated from the shared contract (`task gen`): if the backend
// changes a field, this app stops compiling where it uses that field.
import createClient from "openapi-fetch";

import { getToken } from "../auth";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];
export type Candidate = Schemas["Candidate"];
export type CandidateSummary = Schemas["CandidateSummary"];
export type Compensation = Schemas["Compensation"];
export type Company = Schemas["Company"];
export type CompanySummary = Schemas["CompanySummary"];
export type Contact = Schemas["Contact"];
export type ContactSummary = Schemas["ContactSummary"];
export type JobOrder = Schemas["JobOrder"];
export type JobOrderSummary = Schemas["JobOrderSummary"];
export type Application = Schemas["Application"];
export type Note = Schemas["Note"];
export type ActivityEntry = Schemas["ActivityEntry"];
export type Document = Schemas["Document"];
export type Me = Schemas["Me"];
export type Permission = Schemas["Permission"];
export type ErrorBody = Schemas["Error"];

export const api = createClient<paths>({ baseUrl: "" });

api.use({
  onRequest({ request }) {
    const token = getToken();
    if (token) request.headers.set("Authorization", `Bearer ${token}`);
    return request;
  },
});

/** An error response from the API, with its stable code and request ID. */
export class ApiError extends Error {
  readonly code: Schemas["ErrorCode"];
  readonly requestId: string;
  readonly details: Record<string, unknown>;

  constructor(body: ErrorBody) {
    super(body.error.message);
    this.name = "ApiError";
    this.code = body.error.code;
    this.requestId = body.error.requestId;
    this.details = body.error.details ?? {};
  }
}

/** Returns the data of a call, or throws ApiError for an error response. */
export function unwrap<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.data !== undefined) return result.data;
  // 204 No Content (a delete): done, with nothing to return.
  if (result.response.status === 204) return undefined as T;
  if (isErrorBody(result.error)) throw new ApiError(result.error);
  throw new Error(`Unexpected response ${String(result.response.status)}`);
}

function isErrorBody(value: unknown): value is ErrorBody {
  return typeof value === "object" && value !== null && "error" in value;
}
