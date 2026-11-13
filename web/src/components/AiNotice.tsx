// A notice across the app while the AI the company uses is not answering
// (DeepSeek down, a refused key, no credit): people know why CV reading, fit
// scores, formatting and the assistant are not working, instead of waiting.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { formatWhen } from "../labels";

export const aiStatusKey = ["ai-status"] as const;
const HIDDEN_KEY = "hiringplug.ai-notice.hidden";

/** The failure the person closed the notice for (it comes back for a new one). */
function hiddenSince(): string {
  try { return sessionStorage.getItem(HIDDEN_KEY) ?? ""; } catch { return ""; }
}

/** "The AI provider could not be reached" reads as a sentence in the notice. */
export function sentence(message: string): string {
  const s = message.trim().replace(/\.$/, "");
  return s ? s.charAt(0).toUpperCase() + s.slice(1) + "." : "";
}

export function AiNotice({ permissions }: { permissions: Permission[] }) {
  const status = useQuery({
    queryKey: aiStatusKey,
    queryFn: async () => unwrap(await api.GET("/v1/ai-status")),
    refetchInterval: 60_000, // it goes by itself once the AI answers again
  });
  const [hidden, setHidden] = useState(hiddenSince);
  const s = status.data;
  if (!s?.down) return null;
  const since = s.since ?? s.failedAt ?? "";
  if (hidden && hidden === since) return null;
  const close = () => {
    setHidden(since);
    try { sessionStorage.setItem(HIDDEN_KEY, since); } catch { /* private window: closed for now only */ }
  };
  return (
    <div className="ai-notice" role="alert">
      <div>
        <strong>{s.provider ?? "The AI"} is not answering{since ? ` (since ${formatWhen(since)})` : ""}.</strong>{" "}
        {s.message ? `${sentence(s.message)} ` : ""}
        Reading CVs with AI, fit scores, CV formatting and the assistant will not work until it is back; nothing is lost, try again later.
        {permissions.includes("aiproviders:manage") && <> <a href="#/ai-provider">Check the AI provider</a></>}
      </div>
      <button type="button" className="link" onClick={close} aria-label="Close the AI notice">Close</button>
    </div>
  );
}
