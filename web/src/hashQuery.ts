/** A value from the query part of the page address, e.g. "q" in "#/candidates?q=edran" ("" when absent). */
export function hashParam(name: string, hash = window.location.hash): string {
  const at = hash.indexOf("?");
  return at < 0 ? "" : new URLSearchParams(hash.slice(at + 1)).get(name) ?? "";
}
