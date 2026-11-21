import { describe, expect, it } from "vitest";

import { joinLocation, joinOf, linkProblem, serviceOf, splitLocation, TEAMS_LOCATION } from "./meeting";

describe("online meetings", () => {
  it("knows each service by its link's host, and nothing else", () => {
    expect(serviceOf("https://teams.microsoft.com/l/meetup-join/19%3a")).toBe("teams");
    expect(serviceOf("https://meet.google.com/abc-defg-hij")).toBe("google_meet");
    expect(serviceOf("https://us02web.zoom.us/j/81?pwd=x")).toBe("zoom");
    expect(serviceOf("https://zoom.us.evil.test/j/1")).toBeNull(); // a look-alike host
    expect(serviceOf("http://meet.google.com/abc")).toBeNull(); // not https
    expect(serviceOf("Room 3")).toBeNull();
  });

  it("says why a pasted link will not do", () => {
    expect(linkProblem("zoom", "")).toBe("");
    expect(linkProblem("zoom", "zoom.us/j/1")).toBe("Paste the whole link, starting with https://");
    expect(linkProblem("google_meet", "https://zoom.us/j/1")).toBe("This is not a Google Meet link.");
    expect(linkProblem("teams", "https://teams.live.com/meet/9")).toBe("");
  });

  it("keeps the place and the meeting apart in one location, as Outlook does", () => {
    expect(joinLocation("Room 3", "google_meet", "https://meet.google.com/a")).toBe("Room 3; https://meet.google.com/a");
    expect(splitLocation("Room 3; https://meet.google.com/a")).toEqual({ place: "Room 3", online: "google_meet", link: "https://meet.google.com/a" });
    expect(joinLocation("", "teams", "")).toBe(TEAMS_LOCATION);
    expect(splitLocation(TEAMS_LOCATION)).toEqual({ place: "", online: "teams", link: "" });
    expect(joinLocation("Room 3", "none", "https://meet.google.com/a")).toBe("Room 3");
    expect(splitLocation("Level 12; Menara Maju")).toEqual({ place: "Level 12; Menara Maju", online: "none", link: "" });
    expect(joinOf("Room 3; https://zoom.us/j/1")).toEqual({ service: "zoom", url: "https://zoom.us/j/1" });
    expect(joinOf(TEAMS_LOCATION)).toBeNull(); // no link until Outlook makes it
  });
});
