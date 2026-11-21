// Online meetings on appointments and interviews: Microsoft Teams, Google
// Meet or Zoom. The link lives in the location, as Outlook keeps it (several
// locations are separated by ";"), so Outlook and the candidate's calendar show
// a Join button. A Teams meeting Outlook makes has no link until it is sent:
// its location is Outlook's own "Microsoft Teams Meeting".

export type Service = "teams" | "google_meet" | "zoom";
export type Online = Service | "none";

export const SERVICES: Service[] = ["teams", "google_meet", "zoom"];
export const SERVICE_LABEL: Record<Service, string> = { teams: "Microsoft Teams", google_meet: "Google Meet", zoom: "Zoom" };
export const SHORT_LABEL: Record<Service, string> = { teams: "Teams", google_meet: "Google Meet", zoom: "Zoom" };
export const TEAMS_LOCATION = "Microsoft Teams Meeting";

/** Where a new meeting is made, to paste its link back. */
export const NEW_MEETING: Record<Service, { url: string; label: string }> = {
  teams: { url: "https://teams.microsoft.com/l/meeting/new", label: "New Teams meeting" },
  google_meet: { url: "https://meet.google.com/new", label: "New Google Meet" },
  zoom: { url: "https://zoom.us/meeting/schedule", label: "Schedule in Zoom" },
};

const HOSTS: Record<Service, (host: string) => boolean> = {
  teams: (h) => h === "teams.microsoft.com" || h === "teams.live.com",
  google_meet: (h) => h === "meet.google.com",
  zoom: (h) => h === "zoom.us" || h.endsWith(".zoom.us") || h === "zoom.com" || h.endsWith(".zoom.com"),
};

function parse(link: string): URL | null {
  try {
    const url = new URL(link.trim());
    return url.protocol === "https:" ? url : null;
  } catch {
    return null;
  }
}

/** The service a meeting link is for, or null for any other text. */
export function serviceOf(link: string): Service | null {
  const url = parse(link);
  if (!url) return null;
  const host = url.hostname.toLowerCase();
  return SERVICES.find((s) => HOSTS[s](host)) ?? null;
}

/** Why a link pasted for a service will not do ("" when it will, or when there is none). */
export function linkProblem(service: Service, link: string): string {
  const text = link.trim();
  if (!text) return "";
  if (!parse(text)) return "Paste the whole link, starting with https://";
  return serviceOf(text) === service ? "" : `This is not a ${SERVICE_LABEL[service]} link.`;
}

/** A location split into the place and the online meeting it holds. */
export function splitLocation(location: string | null | undefined): { place: string; online: Online; link: string } {
  const parts = (location ?? "").split(";").map((p) => p.trim()).filter(Boolean);
  let online: Online = "none";
  let link = "";
  const place: string[] = [];
  for (const part of parts) {
    const service: Service | null = online === "none" ? serviceOf(part) : null;
    if (service) {
      online = service;
      link = part;
    } else if (online === "none" && part.toLowerCase() === TEAMS_LOCATION.toLowerCase()) {
      online = "teams";
    } else {
      place.push(part);
    }
  }
  return { place: place.join("; "), online, link };
}

/** The location for a place and an online meeting (a Teams meeting without a link is Outlook's own text). */
export function joinLocation(place: string, online: Online, link: string): string {
  const meeting = online === "none" ? "" : link.trim() || (online === "teams" ? TEAMS_LOCATION : "");
  return [place.trim(), meeting].filter(Boolean).join("; ");
}

/** The join link a location holds, with its service, to show a Join button. */
export function joinOf(location: string | null | undefined): { service: Service; url: string } | null {
  const { online, link } = splitLocation(location);
  return online !== "none" && link ? { service: online, url: link } : null;
}
