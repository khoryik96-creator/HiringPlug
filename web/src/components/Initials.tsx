import type { Availability } from "./candidateLook";
import { statusLook } from "./candidateLook";

/** A person as a coloured circle with their initials (the colour follows the name). */
const AVATARS = ["#7352d8", "#0891b2", "#f97316", "#db2777", "#16a34a", "#6366f1", "#ca8a04"];

/**
 * status: for a candidate, their status (Looking out, Placed...) colours the
 * circle, as on their page; null when they have none. Left out for colleagues.
 */
export function Initials({ name, status }: { name: string; status?: Availability | null }) {
  const letters = name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("") || "?";
  if (status !== undefined) {
    const look = statusLook(status);
    return (
      <span className="avatar status-avatar" style={{ background: look.bg, color: "#1F2937", boxShadow: `inset 0 0 0 2px ${look.edge}` }}
        title={`${name} · ${look.label}`} aria-label={`${name} (status: ${look.label})`}>{letters}</span>
    );
  }
  let sum = 0;
  for (let i = 0; i < name.length; i++) sum += name.charCodeAt(i);
  const color = AVATARS[sum % AVATARS.length];
  return <span className="avatar" style={{ background: color }} title={name} aria-label={name}>{letters}</span>;
}
