// The in-app help: short topics on how to do the main things, kept with the
// app so they change with it. Each topic (and each part of one) names the
// permissions it needs, so people only read about what they can use, and its
// links are shown only when the route table lets them open the page.
import type { Permission } from "../api/client";
import { resolve, ROUTES } from "../routes";

/** Any one of these permissions is enough; none: everyone signed in. */
export type Needs = readonly Permission[];

export interface HelpLink { label: string; href: string }

export interface HelpSection {
  heading: string;
  /** Short sentences, one idea each, in the app's own words. */
  points: string[];
  needs?: Needs;
}

export interface HelpTopic {
  /** Its address: #/help/<id>. */
  id: string;
  title: string;
  /** One line under the title in the list. */
  summary: string;
  /** Other words people may search with (synonyms, old names). */
  words: string;
  needs?: Needs;
  /** The screens it describes, shown as buttons. */
  links: HelpLink[];
  sections: HelpSection[];
}

const has = (needs: Needs | undefined, permissions: readonly Permission[]) =>
  !needs || needs.length === 0 || needs.some((p) => permissions.includes(p));

/**
 * Whether this user may open the page an address names, not a page shown in
 * its place: their start page, or a broader page whose address starts the
 * same ("#/candidates" for "#/candidates/ai-search"). Another version of the
 * same address (AI provider for those who may not set it up) counts.
 */
export function opens(href: string, permissions: readonly Permission[]): boolean {
  const path = href.split("?")[0] ?? href;
  const { route } = resolve(href, permissions);
  if (!route.path.test(path)) return false;
  return !ROUTES.slice(0, ROUTES.indexOf(route)).some((r) => r.path.test(path) && r.path.source !== route.path.source);
}

/** The topics this user can use: parts and links they cannot use left out, empty topics dropped. */
export function visibleTopics(topics: readonly HelpTopic[], permissions: readonly Permission[]): HelpTopic[] {
  return topics.flatMap((t) => {
    if (!has(t.needs, permissions)) return [];
    const sections = t.sections.filter((s) => has(s.needs, permissions));
    if (sections.length === 0) return [];
    return [{ ...t, sections, links: t.links.filter((l) => opens(l.href, permissions)) }];
  });
}

const normalize = (value: string) =>
  value.normalize("NFD").replace(/\p{Diacritic}/gu, "").toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();

const stopWords = new Set(
  "a an i me my can could how do does where what who which want to the for in on of and with is it are be".split(" "),
);

const wordsOf = (text: string) => normalize(text).split(" ").filter(Boolean);

/** How well one search word matches a list of words: whole word 2, its start 1. */
function hit(token: string, words: string[]): number {
  if (words.includes(token)) return 2;
  return words.some((w) => w.startsWith(token)) ? 1 : 0;
}

export interface HelpHit {
  topic: HelpTopic;
  /** The first sentence that has the words (none when only the title or summary does). */
  snippet: string | null;
}

/**
 * The topics with every word searched for (ignoring case, accents and words
 * like "how" and "to"), best first: words in the title count most, then the
 * summary and search words, then the text.
 */
export function searchTopics(topics: readonly HelpTopic[], query: string): HelpHit[] {
  const tokens = [...new Set(wordsOf(query).filter((t) => !stopWords.has(t)))];
  if (tokens.length === 0) return [];
  const scored = topics.flatMap((topic) => {
    const title = wordsOf(topic.title);
    const about = wordsOf(`${topic.summary} ${topic.words}`);
    const sentences = topic.sections.flatMap((s) => [s.heading, ...s.points]);
    const body = wordsOf(sentences.join(" "));
    let score = 0;
    for (const token of tokens) {
      const inTitle = hit(token, title);
      const inAbout = hit(token, about);
      const inBody = hit(token, body);
      if (!inTitle && !inAbout && !inBody) return [];
      score += inTitle * 6 + inAbout * 3 + inBody;
    }
    // A sentence rather than a heading, unless only a heading has the words.
    const best = [...topic.sections.flatMap((s) => s.points), ...topic.sections.map((s) => s.heading)]
      .map((text, i) => ({ text, i, n: tokens.filter((t) => hit(t, wordsOf(text)) > 0).length }))
      .filter((s) => s.n > 0)
      .sort((a, b) => b.n - a.n || a.i - b.i)[0];
    return [{ topic, snippet: best?.text ?? null, score }];
  });
  return scored.sort((a, b) => b.score - a.score).map(({ topic, snippet }) => ({ topic, snippet }));
}
