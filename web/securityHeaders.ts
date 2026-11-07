// The web app's security headers, read from deploy/security-headers.conf
// (nginx's), so `vite preview` (the browser smoke tests) serves the app under
// the same policy as production.
import { readFileSync } from "node:fs";

export function securityHeaders(path: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of readFileSync(path, "utf8").matchAll(/^add_header\s+(\S+)\s+"([^"]*)"/gm)) {
    if (m[1] && m[2] !== undefined) out[m[1]] = m[2];
  }
  return out;
}
