/** A person as a coloured circle with their initials (the colour follows the name). */
const AVATARS = ["#7352d8", "#0891b2", "#f97316", "#db2777", "#16a34a", "#6366f1", "#ca8a04"];

export function Initials({ name }: { name: string }) {
  const letters = name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? "").join("") || "?";
  let sum = 0;
  for (let i = 0; i < name.length; i++) sum += name.charCodeAt(i);
  const color = AVATARS[sum % AVATARS.length];
  return <span className="avatar" style={{ background: color }} title={name} aria-label={name}>{letters}</span>;
}
