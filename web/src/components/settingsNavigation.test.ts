import { describe, expect, it } from "vitest";

import type { Permission } from "../api/client";
import { ROUTES, menu } from "../routes";
import { groupSettings, searchSettings, settingsGroups, settingsSearchEntries } from "./settingsNavigation";

const entriesFor = (permissions: Permission[]) => settingsSearchEntries(groupSettings(menu("settings", permissions)), permissions);

describe("Settings navigation", () => {
  it("places every Settings route in one group, including alternate routes for one section", () => {
    const expected = [...new Set(ROUTES.filter((route) => route.menu?.in === "settings").map((route) => route.section))];
    expect(settingsGroups.flatMap((group) => group.sections).sort()).toEqual(expected.sort());
  });

  it("ranks plain-language matches and links to the right subsection", () => {
    const entries = entriesFor(["settings:manage", "emails:send", "calendars:use", "intake:create"]);
    const href = (query: string) => searchSettings(entries, query)[0]?.href;
    expect(href("who can see salary?")).toBe("#/settings/salary#salary-in-emails");
    expect(href("How do I connect Outlook?")).toBe("#/email-connection");
    expect(href("change notice period")).toBe("#/settings/lists#notice-period");
    expect(href("calendar")).toBe("#/calendar");
    expect(href("upload CVs")).toBe("#/import");
    expect(href("nonsense")).toBeUndefined();
  });

  it("only indexes permitted Settings pages and usable feature shortcuts", () => {
    const entries = entriesFor([]);
    expect(entries.map(({ href }) => href)).toEqual(["#/account", "#/my-dropdowns", "#/ai-provider", "#/companion", "#/help"]);
    expect(searchSettings(entries, "who can see salary")).toEqual([]);
    expect(searchSettings(entries, "Outlook")).toEqual([]);
    expect(searchSettings(entries, "upload CVs")).toEqual([]);
    expect(searchSettings(entriesFor(["emails:send"]), "Outlook Mail")[0]?.href).toBe("#/outlook");
    expect(searchSettings(entries, "help")[0]?.href, "the help, for everyone").toBe("#/help");
  });
});
