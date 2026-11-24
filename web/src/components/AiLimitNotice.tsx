// A notice across the app once the company has used this month's whole AI
// limit: everyone knows why CV reading, fit scores, formatting, the
// assistant and the other AI features stopped (they refuse, nothing waits),
// and until when. Those who may see the company's AI use
// get a link to it. Closing it hides it for this month, in this session.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";

export const aiUsageStatusKey = ["ai-usage-status"] as const;
const HIDDEN_KEY = "hiringplug.ai-limit-notice.hidden";

/** The month (its reset time) the person closed the notice for. */
function hiddenFor(): string {
  try { return sessionStorage.getItem(HIDDEN_KEY) ?? ""; } catch { return ""; }
}

/**
 * The day the limit starts again, e.g. "1 Dec": always the first of a month
 * in the agency's time, whatever the browser's zone (a zone is at most 14
 * hours off, so 14 hours later is in that month everywhere).
 */
export function resetDay(resetsAt: string): string {
  const later = new Date(Date.parse(resetsAt) + 14 * 3600_000);
  const first = new Date(Date.UTC(later.getUTCFullYear(), later.getUTCMonth(), 1));
  return first.toLocaleDateString("en-GB", { day: "numeric", month: "short", timeZone: "UTC" });
}

export function AiLimitNotice({ permissions }: { permissions: Permission[] }) {
  const status = useQuery({
    queryKey: aiUsageStatusKey,
    queryFn: async () => unwrap(await api.GET("/v1/ai-usage/status")),
    refetchInterval: 60_000, // it goes by itself when a new month starts or the limit is raised
  });
  const [hidden, setHidden] = useState(hiddenFor);
  const s = status.data;
  if (!s?.reached) return null;
  if (hidden && hidden === s.resetsAt) return null;
  const close = () => {
    setHidden(s.resetsAt);
    try { sessionStorage.setItem(HIDDEN_KEY, s.resetsAt); } catch { /* private window: closed for now only */ }
  };
  return (
    <div className="ai-notice ai-limit-notice" role="alert">
      <div>
        <strong>This month&apos;s AI limit is reached: AI features are paused until {resetDay(s.resetsAt)}.</strong>{" "}
        Until then, CVs are read without the AI, and fit scores, CV formatting, the assistant and the other AI features are
        off: try them again from {resetDay(s.resetsAt)}.
        {permissions.includes("aiusage:read") && <> <a href="#/settings/ai-usage">See the company&apos;s AI use</a></>}
      </div>
      <button type="button" className="link" onClick={close} aria-label="Close the AI limit notice">Close</button>
    </div>
  );
}
