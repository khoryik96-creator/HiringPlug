// Dates the claims screens offer are the user's own: in Kuala Lumpur at
// 07:30 (23:30 UTC the day before) today's receipt can be dated today.
import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { examplePath } from "../../pages/ClaimsStorage";
import { RECRUITER, renderWithClient, testClaim } from "../../test-utils";
import { ClaimRow } from "./ClaimRow";

const zone = process.env.TZ;
beforeEach(() => {
  process.env.TZ = "Asia/Kuala_Lumpur";
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-11-19T23:30:00Z"));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  process.env.TZ = zone;
});

it("lets today's receipt be dated today, early in the morning", () => {
  renderWithClient(<table><tbody><ClaimRow claim={testClaim({ status: "draft" })} permissions={RECRUITER} selected={false}
    onSelect={vi.fn()} onView={vi.fn()} onDelete={vi.fn()} /></tbody></table>);
  expect(screen.getByLabelText<HTMLInputElement>("Receipt date").max).toBe("2026-11-20");
});

it("names the storage folder with the user's date", () => {
  expect(examplePath("Claims")).toBe("Claims/2026-11/Aisyah Rahman/Grab fare/2026-11-20 Grab MYR 23.00.pdf");
});
