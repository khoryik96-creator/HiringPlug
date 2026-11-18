import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { fakeCompanion } from "../test-utils";
import { Companion, COMPANION_LATEST } from "./Companion";

let stop: () => void = () => undefined;
afterEach(() => { stop(); cleanup(); vi.useRealTimers(); });

describe("LinkedIn companion page", () => {
  it("offers the download, and says when this browser has no companion yet", async () => {
    render(<Companion />);
    const link = screen.getByRole("link", { name: /Download the companion/ });
    expect(link.getAttribute("href")).toBe("/linkedin-companion.zip");
    expect(link.hasAttribute("download")).toBe(true);
    expect(await screen.findByText("Not connected in this browser yet.", {}, { timeout: 3000 })).toBeTruthy();
    expect(screen.getByText(/Load unpacked/)).toBeTruthy();
  });

  it("says it is connected, with its version, when it is up to date", async () => {
    stop = fakeCompanion(() => null, COMPANION_LATEST).stop;
    render(<Companion />);
    expect(await screen.findByText(`Connected in this browser (version ${COMPANION_LATEST}).`)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Update the companion/ })).toBeNull();
  });

  it("asks a companion before self-updating for a new download, one last time", async () => {
    stop = fakeCompanion(() => null, "1.0.3").stop;
    render(<Companion />);
    expect(await screen.findByText(/version 1.0.3 is old: download it again/)).toBeTruthy();
    expect(screen.getByText(/updates itself/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Update the companion/ })).toBeNull();
  });

  it("opens the companion's own update page when a newer version is ready", async () => {
    expect(COMPANION_LATEST).toBe(__COMPANION_VERSION__); // the version built with this HiringPlug
    const fake = fakeCompanion((action) => (action === "openUpdater" ? { result: true } : null), "1.4.0");
    stop = fake.stop;
    render(<Companion latest="1.5.0" />);
    expect(await screen.findByText("Connected (version 1.4.0). Version 1.5.0 is ready: update it here.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Update the companion/ }));
    expect(await screen.findByText(/update page opened in a new tab/)).toBeTruthy();
    expect(fake.asked.map((a) => a.action)).toEqual(["openUpdater"]);
  });
});
