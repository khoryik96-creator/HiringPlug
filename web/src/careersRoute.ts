// The public careers page lives at /careers/<company>[/<ad>] (real addresses
// that search engines and job boards can link to) and, for older links, at
// #/careers/<company>[/<ad>]. A ?via=<board> on either says which job board
// link the visitor came from.

export interface CareersRoute {
  slug: string;
  adId: string | undefined;
  via: string | undefined;
}

const ROUTE = /^\/careers\/([a-z0-9]+(?:-[a-z0-9]+)*)(?:\/(ad_[0-9a-z]+))?\/?$/;

export function careersRoute(loc: Pick<Location, "pathname" | "search" | "hash">): CareersRoute | null {
  const [hashPath = "", hashQuery = ""] = loc.hash.replace(/^#/, "").split("?");
  const fromHash = ROUTE.exec(hashPath);
  const fromPath = fromHash ? null : ROUTE.exec(loc.pathname);
  const m = fromHash ?? fromPath;
  if (!m?.[1]) return null;
  const query = new URLSearchParams(fromHash ? hashQuery : loc.search);
  return { slug: m[1], adId: m[2], via: query.get("via") ?? undefined };
}

/** The address to share for a company's page or one ad, optionally for one job board. */
export function careersUrl(origin: string, slug: string, adId?: string, via?: string): string {
  return `${origin}/careers/${slug}${adId ? `/${adId}` : ""}${via ? `?via=${via}` : ""}`;
}
