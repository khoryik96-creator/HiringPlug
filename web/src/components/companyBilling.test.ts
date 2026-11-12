import { expect, it } from "vitest";

import { billingText } from "./companyBilling";

it("writes a client's billing in whole ringgit, each currency apart", () => {
  expect(billingText([{ currency: "MYR", total: "30000.00", notInvoiced: "0", invoiced: "0", paid: "30000.00" }])).toBe("RM 30,000");
  expect(billingText([
    { currency: "MYR", total: "12500.50", notInvoiced: "0", invoiced: "0", paid: "0" },
    { currency: "SGD", total: "3500.00", notInvoiced: "0", invoiced: "0", paid: "0" },
  ])).toBe("RM 12,501 + SGD 3,500");
  expect(billingText([]), "nothing billed").toBeNull();
  expect(billingText(undefined)).toBeNull();
  expect(billingText([{ currency: "MYR", total: "0.00", notInvoiced: "0", invoiced: "0", paid: "0" }])).toBeNull();
});
