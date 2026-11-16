import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { fakeCompanion } from "../test-utils";
import { Companion } from "./Companion";

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

  it("says it is connected, with its version", async () => {
    stop = fakeCompanion(() => null, "1.1.0").stop;
    render(<Companion />);
    expect(await screen.findByText("Connected in this browser (version 1.1.0).")).toBeTruthy();
  });

  it("asks for a new download when the companion is too old", async () => {
    stop = fakeCompanion(() => null, "1.0.3").stop;
    render(<Companion />);
    expect(await screen.findByText(/version 1.0.3 is old: download it again/)).toBeTruthy();
  });
});
