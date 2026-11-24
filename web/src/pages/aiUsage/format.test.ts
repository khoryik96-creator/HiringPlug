import { describe, expect, it } from "vitest";

import { aiMoney, fromTenThousandths, groupDigits, limitState, monthName, monthShort, tenThousandths } from "./format";
import { bodyOf, draftOf } from "./limitFormState";
import { niceCeil } from "./MonthsChart";

describe("AI usage numbers", () => {
  it("keeps money exactly as the API sent it", () => {
    expect(groupDigits("1234567.0001")).toBe("1,234,567.0001");
    expect(groupDigits("999.10")).toBe("999.10");
    expect(aiMoney("0.0003", "MYR")).toBe("MYR 0.0003");
    expect(aiMoney("10000000000000000.0001", "USD")).toBe("USD 10,000,000,000,000,000.0001"); // past float precision
  });

  it("turns amounts into whole ten-thousandths and back for the chart", () => {
    expect(tenThousandths("3.84")).toBe(38_400);
    expect(tenThousandths("0.0001")).toBe(1);
    expect(tenThousandths("12")).toBe(120_000);
    expect(fromTenThousandths(50_000)).toBe("5");
    expect(fromTenThousandths(25_000)).toBe("2.5");
    expect(fromTenThousandths(1)).toBe("0.0001");
    expect([niceCeil(38_400), niceCeil(100_000), niceCeil(120_001), niceCeil(0)]).toEqual([50_000, 100_000, 200_000, 1]);
  });

  it("names months whatever the browser's time zone", () => {
    expect(monthName("2026-01")).toBe("January 2026");
    expect(monthShort("2025-12")).toBe("Dec");
  });

  it("says how close the month is to its limit", () => {
    expect(limitState({ reached: false, usedPercent: 79, warnPercent: 80 }).label).toBe("Within limit");
    expect(limitState({ reached: false, usedPercent: 80, warnPercent: 80 }).label).toBe("Near the limit");
    expect(limitState({ reached: true, usedPercent: 100, warnPercent: 80 }).label).toBe("Limit reached");
  });

  it("checks whole numbers before sending", () => {
    const d = draftOf({ limitKind: "tokens", limitTokens: 5000, serverDefaultTokens: 2_000_000, limitMoney: null, currency: "USD",
      priceInputPerMillion: null, priceOutputPerMillion: null, warnPercent: 80, version: 1, updatedAt: null });
    expect(bodyOf({ ...d, tokens: "1,500,000" }, 1).limitTokens).toBe(1_500_000);
    expect(() => bodyOf({ ...d, tokens: "" }, 1)).toThrow("Some fields need attention.");
    expect(bodyOf({ ...d, tokens: "", useDefault: true }, 1).limitTokens).toBeNull();
  });
});
