export function localInput(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}T${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}
export function startDay(date: Date): Date { return new Date(date.getFullYear(), date.getMonth(), date.getDate()); }
export function addDays(date: Date, days: number): Date { const next = new Date(date); next.setDate(next.getDate() + days); return next; }
/** The day after a "YYYY-MM-DD" day, as one (calendar arithmetic, no time zone). */
export function nextDay(day: string): string {
  const d = new Date(`${day}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 1);
  return d.toISOString().slice(0, 10);
}
export function sameDay(a: Date, b: Date): boolean { return startDay(a).getTime() === startDay(b).getTime(); }
export type CalendarView = "month" | "week" | "day" | "agenda";
export function daysFor(date: Date, mode: CalendarView): Date[] {
  if (mode === "day") return [startDay(date)];
  const first = mode === "month" ? new Date(date.getFullYear(), date.getMonth(), 1) : startDay(date);
  if (mode !== "agenda") first.setDate(first.getDate() - (first.getDay() + 6) % 7);
  return Array.from({ length: mode === "month" ? 42 : 7 }, (_, i) => addDays(first, i));
}
export const dateLabel = (date: Date) => date.toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" });
export const timeLabel = (date: string) => new Date(date).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });

/** Browser-independent wall time in an IANA zone, including across DST changes. */
export function zonedInput(date: Date, zone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(date);
  const get = (key: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === key)?.value ?? "";
  return `${get("year")}-${get("month")}-${get("day")}T${get("hour")}:${get("minute")}`;
}
export function wallTime(value: string, zone: string): Date {
  const target = Date.parse(`${value}:00Z`);
  if (!Number.isFinite(target)) throw new Error("Choose a valid date and time.");
  const offsets = new Set([-86400000, 0, 86400000].map((shift) => {
    const sample = target + shift;
    return Date.parse(`${zonedInput(new Date(sample), zone)}:00Z`) - sample;
  }));
  const matches = [...offsets].map((offset) => new Date(target - offset)).filter((candidate) => zonedInput(candidate, zone) === value);
  if (matches.length === 0) throw new Error("This local time does not exist because the clocks change. Choose another time.");
  if (matches.length > 1) throw new Error("This local time occurs twice when the clocks change. Choose a time outside that transition.");
  const result = matches[0];
  if (!result) throw new Error("Choose a valid date and time.");
  return result;
}
