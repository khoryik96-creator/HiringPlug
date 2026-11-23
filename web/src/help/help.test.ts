import { describe, expect, it } from "vitest";

import type { Permission } from "../api/client";
import { resolve } from "../routes";
import { opens, searchTopics, visibleTopics, type HelpTopic } from "./help";
import { HELP_TOPICS } from "./topics";

// An owner's permissions (every one the help mentions).
const OWNER = [...new Set(HELP_TOPICS.flatMap((t) => [...(t.needs ?? []), ...t.sections.flatMap((s) => s.needs ?? [])]))]
  .concat(["pipeline:read", "settings:manage", "identity.team:manage", "identity.roles:manage", "aiproviders:manage",
    "emails:send", "placements:read", "automations:read", "claims:create", "leads:read", "calendars:use", "folders:use"] as Permission[]);
const VIEWER: Permission[] = ["candidates:read", "companies:read", "contacts:read", "jobs:read", "pipeline:read", "notifications:read"];

const ids = (topics: HelpTopic[]) => topics.map((t) => t.id);

describe("help topics", () => {
  it("each has its own address, words, links and something to read", () => {
    expect(new Set(ids(HELP_TOPICS)).size).toBe(HELP_TOPICS.length);
    for (const t of HELP_TOPICS) {
      expect(t.id, t.id).toMatch(/^[a-z-]+$/);
      expect(resolve(`#/help/${t.id}`, VIEWER).route.section, t.id).toBe("help");
      expect(t.sections.length, t.id).toBeGreaterThan(0);
      for (const s of t.sections) expect(s.points.length, `${t.id}: ${s.heading}`).toBeGreaterThan(0);
    }
  });

  it("links only to pages that exist (for someone who may open them all)", () => {
    for (const t of HELP_TOPICS) {
      for (const l of t.links) expect(opens(l.href, OWNER), `${t.id} → ${l.href}`).toBe(true);
    }
  });

  it("covers the main things a recruiter asks about", () => {
    const all = ids(HELP_TOPICS);
    for (const id of ["add-candidates", "search", "jobs-pipeline", "interviews-calendar", "offers-placements", "emails", "whatsapp",
      "folders", "duplicates", "notifications", "exports", "reports", "team-roles"]) expect(all).toContain(id);
  });
});

describe("what each role sees", () => {
  it("hides topics, parts and links the role cannot use", () => {
    const owner = visibleTopics(HELP_TOPICS, OWNER);
    const viewer = visibleTopics(HELP_TOPICS, VIEWER);
    expect(ids(owner)).toContain("exports");
    expect(ids(owner)).toContain("add-candidates");
    expect(ids(viewer)).not.toContain("exports"); // no export permission
    expect(ids(viewer)).not.toContain("add-candidates");
    expect(ids(viewer)).not.toContain("emails");
    expect(ids(viewer)).toContain("search");

    const ownerTeam = owner.find((t) => t.id === "team-roles");
    const viewerTeam = viewer.find((t) => t.id === "team-roles");
    expect(ownerTeam?.sections.map((s) => s.heading)).toContain("Invite your team");
    expect(viewerTeam?.sections.map((s) => s.heading)).toEqual(["Roles"]);
    expect(viewerTeam?.links, "Team and Settings pages are not theirs to open").toEqual([]);

    const viewerSearch = viewer.find((t) => t.id === "search");
    expect(viewerSearch?.sections.map((s) => s.heading)).not.toContain("AI search");
    expect(viewerSearch?.links.map((l) => l.href)).toEqual(["#/candidates"]);
  });

  it("links only to the page named, never one shown in its place", () => {
    expect(opens("#/candidates", VIEWER)).toBe(true);
    expect(opens("#/candidates/ai-search", VIEWER), "would show the candidate list").toBe(false);
    expect(opens("#/team", VIEWER), "would show the start page").toBe(false);
    expect(opens("#/ai-provider", VIEWER), "the page saying who sets it up").toBe(true);
    expect(opens("#/claims", ["claims:approve"]), "approvers who do not claim").toBe(true);
  });

  it("tells someone who cannot set up the AI who can", () => {
    const ai = (p: Permission[]) => visibleTopics(HELP_TOPICS, p).find((t) => t.id === "ai")?.sections.map((s) => s.heading);
    expect(ai(VIEWER)).not.toContain("Switch it on");
    expect(ai(OWNER)).toContain("Switch it on");
  });

  it("drops a topic whose every part needs something else", () => {
    const topic: HelpTopic = { id: "x", title: "X", summary: "", words: "", links: [], sections: [{ heading: "H", points: ["p"], needs: ["bin:manage"] }] };
    expect(visibleTopics([topic], VIEWER)).toEqual([]);
    expect(visibleTopics([topic], ["bin:manage"])).toHaveLength(1);
  });
});

describe("searching the help", () => {
  const topics = visibleTopics(HELP_TOPICS, OWNER);
  const found = (q: string) => searchTopics(topics, q).map((h) => h.topic.id);

  it("finds a topic by its words in any order, ignoring case, accents and words like how and to", () => {
    expect(found("import Excel")[0]).toBe("add-candidates");
    expect(found("EXCEL import")[0]).toBe("add-candidates");
    expect(found("how do I invite my team")[0]).toBe("team-roles");
    expect(found("whatsápp")[0]).toBe("whatsapp");
    expect(found("bool")[0], "the start of a word").toBe("search");
  });

  it("puts topics named by the words first", () => {
    expect(found("export")[0]).toBe("exports");
    expect(found("folders")[0]).toBe("folders");
    expect(found("morning email")[0]).toBe("notifications");
  });

  it("needs every word, and says nothing for nothing", () => {
    expect(found("whatsapp invoice")).toEqual([]);
    expect(found("zebra")).toEqual([]);
    expect(found("")).toEqual([]);
    expect(found("how to")).toEqual([]);
  });

  it("shows the sentence that has the words", () => {
    const [hit] = searchTopics(topics, "xls");
    expect(hit?.snippet).toContain(".xls");
  });

  it("finds only what the role can read", () => {
    expect(searchTopics(visibleTopics(HELP_TOPICS, VIEWER), "export excel")).toEqual([]);
  });
});
