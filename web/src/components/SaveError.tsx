import { ApiError } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";
import { fieldErrors } from "./forms";

/**
 * Why a save failed, at the bottom of an edit form. Messages for fields that
 * no input shows (`shown` lists the ones that are, e.g. "body") are listed
 * here, so a failed save never looks as if nothing happened. A conflict
 * (someone else saved first) offers to load their version.
 */
export function SaveError({ error, shown, onReload, reloadLabel = "Reload their version (your changes here will be lost)" }: {
  error: unknown;
  shown: readonly string[];
  onReload: () => void;
  reloadLabel?: string;
}) {
  if (error === null || error === undefined) return null;
  const unshown = Object.entries(fieldErrors(error)).filter(([field]) => !shown.includes(field));
  const stale = error instanceof ApiError && error.code === "CONFLICT_STALE_VERSION";
  return (
    <div className="full">
      <ErrorMessage error={error}>
        {unshown.length > 0 && (
          <ul>
            {unshown.map(([field, message]) => (
              <li key={field}>
                {field}: {message}
              </li>
            ))}
          </ul>
        )}
        {stale && (
          <button type="button" className="link" onClick={onReload}>
            {reloadLabel}
          </button>
        )}
      </ErrorMessage>
    </div>
  );
}
