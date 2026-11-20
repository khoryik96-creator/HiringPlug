import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { JobTable } from "./JobTable";

afterEach(cleanup);

it("scrolls sideways in its own box on a narrow screen, not the whole page", () => {
  render(<JobTable empty="None" jobs={[{ id: "jo_1", title: "QA and Platform Engineer", companyId: "comp_1", companyName: "Contoh",
    status: "open", location: "Kuala Lumpur", openings: 2, ownerId: null, ownerName: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z" }]} />);
  expect(screen.getByRole("table").parentElement?.className).toBe("table-scroll");
  expect(screen.getByRole("link", { name: "QA and Platform Engineer" }).getAttribute("href")).toBe("#/jobs/jo_1");
});
