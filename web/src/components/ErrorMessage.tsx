import type { ReactNode } from "react";

import { ApiError } from "../api/client";

/**
 * Shows what went wrong, with the request ID support needs to find the logs.
 * `children` adds details or actions inside the same alert.
 */
export function ErrorMessage({ error, children }: { error: unknown; children?: ReactNode }) {
  if (error === null || error === undefined) return null;
  const message = error instanceof Error ? error.message : "Something went wrong.";
  return (
    <div role="alert" className="error">
      <p>{message}</p>
      {children}
      {error instanceof ApiError && (
        <p className="muted">
          Reference: <code>{error.requestId}</code>
        </p>
      )}
    </div>
  );
}
