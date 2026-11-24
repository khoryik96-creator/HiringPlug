// When something happened, short: "Today 15:42", "Yesterday 18:20",
// "Mon 23 Nov 18:20", "23 Nov 2025 18:20". Times are the viewer's clock, as
// everywhere in the app (formatWhen); a salary view's day is the agency's.
import { formatWhen } from "../../labels";
import { localDay } from "./filters";

/** "Today", "Yesterday", "Mon 23 Nov" this year, "23 Nov 2025" before. */
export function dayText(day: string, now = new Date()): string {
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  if (day === localDay(now)) return "Today";
  if (day === localDay(yesterday)) return "Yesterday";
  const d = new Date(`${day}T00:00:00Z`);
  const sameYear = d.getUTCFullYear() === now.getFullYear();
  return d.toLocaleDateString("en-GB", sameYear
    ? { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" }
    : { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" }).replace(",", "");
}

/** "15:42" on the viewer's clock. */
export function timeText(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
}

/** A moment: its day in words, then the time. */
export function whenText(iso: string, now = new Date()): string {
  return `${dayText(localDay(new Date(iso)), now)} ${timeText(iso)}`;
}

/** The full date and time, for a tooltip. */
export const fullWhen = formatWhen;
