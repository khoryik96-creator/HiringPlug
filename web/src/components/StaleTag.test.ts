import { expect, it } from "vitest";

import { isStale } from "./StaleTag";

it("counts a record as stale only once a whole year has passed since it was touched", () => {
  const now = new Date("2026-11-27T10:00:00Z");
  expect(isStale("2025-11-27T09:00:00Z", now)).toBe(true);
  expect(isStale("2025-11-27T11:00:00Z", now)).toBe(false);
  expect(isStale("2026-06-01T00:00:00Z", now)).toBe(false);
});
