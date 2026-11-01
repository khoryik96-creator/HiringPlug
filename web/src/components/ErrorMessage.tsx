import { ApiError } from "../api/client";

/** Shows what went wrong, with the request ID support needs to find the logs. */
export function ErrorMessage({ error }: { error: unknown }) {
  if (error === null || error === undefined) return null;
  const message = error instanceof Error ? error.message : "Something went wrong.";
  return (
    <div role="alert" className="error">
      <p>{message}</p>
      {error instanceof ApiError && (
        <p className="muted">
          Reference: <code>{error.requestId}</code>
        </p>
      )}
    </div>
  );
}
