import { describe, expect, it } from "vitest";

import { guaranteeLabel, pickTier, rangeLabel, rowFromTier, tierFromRow, type Tier } from "./termsTiers";

const rate = (salaryFrom: string | null, salaryBelow: string | null, feePercent: string): Tier =>
  ({ salaryFrom, salaryBelow, feePercent, guaranteeDays: 90, paymentTermsDays: 30, remarks: null });
const tiers = [rate(null, "120000", "18"), rate("120000", null, "25")];

describe("a client's rates", () => {
  it("pick the rate for a salary's yearly amount", () => {
    const pick = (amount: string, period = "month", currency = "MYR") => pickTier(tiers, "MYR", { amount, currency, period });
    expect(pick("9,999")).toBe(0);
    expect(pick("10000")).toBe(1); // RM 120,000 a year is in the upper band
    expect(pick("130000", "year")).toBe(1);
    expect(pick("10000", "month", "SGD")).toBe(-1);
    expect(pick("")).toBe(-1);
    expect(pickTier([rate(null, null, "20")], "MYR", null)).toBe(0); // one rate for any salary
  });

  it("read as people write them", () => {
    expect(rangeLabel(tiers[0] as Tier, "MYR")).toBe("Below RM 120,000");
    expect(rangeLabel(rate("80000", "120000", "20"), "SGD")).toBe("SGD 80,000 to below SGD 120,000");
    expect(guaranteeLabel(90)).toBe("3 months");
    expect(guaranteeLabel(45)).toBe("45 days");
    expect(tierFromRow(rowFromTier(tiers[1] as Tier))).toEqual(tiers[1]);
  });
});
