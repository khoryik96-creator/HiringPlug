// A company's name guessed from its LinkedIn page or website address, for
// the Add company form ("linkedin.com/company/maju-foods" → "Maju Foods").
// The website's own name replaces it once read.

const words = (s: string) => s.split(/[-_.\s]+/).filter(Boolean).map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" ");

/** The name in a linkedin.com/company/… address, or "". */
function fromLinkedin(url: string): string {
  const m = /linkedin\.com\/(?:company|school|showcase)\/([^/?#]+)/i.exec(url);
  if (!m?.[1]) return "";
  let slug = m[1];
  try { slug = decodeURIComponent(slug); } catch { /* keep it as it is */ }
  return /^\d+$/.test(slug) ? "" : words(slug); // a number names no one
}

/** The name in a website address ("www.majufoods.com.my" → "Majufoods"), or "". */
function fromWebsite(url: string): string {
  const host = url.trim().replace(/^[a-z]+:\/\//i, "").split(/[/?#:]/)[0]?.toLowerCase() ?? "";
  const labels = host.split(".").filter((l) => l && l !== "www");
  if (labels.length < 2) return "";
  // The label before the country and kind parts (.com.my, .co.uk, .gov.my…).
  const generic = new Set(["com", "co", "net", "org", "gov", "edu", "biz", "info", "my", "sg", "uk", "id", "ph", "th", "vn", "au", "in", "hk", "io"]);
  let i = labels.length - 1;
  while (i > 0 && generic.has(labels[i] ?? "")) i--;
  return words(labels[i] ?? "");
}

/** The guessed name: from LinkedIn (its words are kept apart) or else the website. */
export function guessCompanyName(website: string, linkedin: string): string {
  return fromLinkedin(linkedin) || fromWebsite(website);
}
