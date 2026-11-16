import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { hasLocalLink, LocalLinkWarning } from "./LocalLinkWarning";

afterEach(cleanup);

describe("links that only work on this computer", () => {
  it("are found in an email's text or HTML", () => {
    for (const text of [
      "Please pick a time that suits you (30 minutes): http://localhost:3000/book/abc",
      '<p><a href="http://127.0.0.1:8080/book/x">book</a></p>',
      "https://localhost/book",
      "http://0.0.0.0:3000/",
    ]) expect(hasLocalLink(text), text).toBe(true);
    for (const text of [
      "https://app.youragency.com/book/abc",
      "http://192.168.1.10:3000/book/abc", // the office network: others there can open it
      "Meet at localhost cafe",
      "https://localhost.example.com/x",
    ]) expect(hasLocalLink(text), text).toBe(false);
  });

  it("are pointed out before sending", () => {
    const shown = render(<LocalLinkWarning html="<p>Pick a time: http://localhost:3000/book/abc</p>" />);
    expect(screen.getByRole("alert").textContent).toContain("only works on this computer");
    shown.rerender(<LocalLinkWarning html="<p>Pick a time: https://app.youragency.com/book/abc</p>" />);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
