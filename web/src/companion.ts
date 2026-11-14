// The HiringPlug LinkedIn companion (extension/linkedin-companion): a browser
// extension that reads LinkedIn in the recruiter's own browser. Its bridge,
// on this page, answers window messages; this is the page's side.
import { useEffect, useState } from "react";

const APP = "hiringplug-app";
const COMPANION = "hiringplug-companion";

interface Reply { source?: string; type?: string; id?: string; result?: unknown; error?: string; version?: string }

const fromCompanion = (e: MessageEvent): Reply | null => {
  const data = e.data as Reply | null;
  return e.source === window && e.origin === window.location.origin && data?.source === COMPANION ? data : null;
};

/** Whether the companion is installed and connected to this HiringPlug (null while asking). */
export function useCompanion(): { ready: boolean | null; version: string | null } {
  const [state, setState] = useState<{ ready: boolean | null; version: string | null }>({ ready: null, version: null });
  useEffect(() => {
    const listen = (e: MessageEvent) => {
      const m = fromCompanion(e);
      if (m?.type === "ready") setState({ ready: true, version: m.version ?? null });
    };
    window.addEventListener("message", listen);
    window.postMessage({ source: APP, type: "hello" }, window.location.origin);
    const timer = setTimeout(() => { setState((s) => (s.ready ? s : { ready: false, version: null })); }, 1500);
    return () => { window.removeEventListener("message", listen); clearTimeout(timer); };
  }, []);
  return state;
}

/** Asks the companion to do something; rejects with its reason. */
export function companionCall<T>(action: string, args: Record<string, unknown>, timeoutMs = 120_000): Promise<T> {
  const id = crypto.randomUUID();
  return new Promise<T>((resolve, reject) => {
    const done = () => { window.removeEventListener("message", listen); clearTimeout(timer); };
    const listen = (e: MessageEvent) => {
      const m = fromCompanion(e);
      if (m?.type !== "reply" || m.id !== id) return;
      done();
      if (m.error) reject(new Error(m.error));
      else resolve(m.result as T);
    };
    const timer = setTimeout(() => { done(); reject(new Error("The LinkedIn companion did not answer. Check the LinkedIn tab, then try again.")); }, timeoutMs);
    window.addEventListener("message", listen);
    window.postMessage({ source: APP, type: "call", id, action, args }, window.location.origin);
  });
}

/** A random operation id for a read that has no server check (the company's number). */
export function operationId(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, "0")).join("");
}
