import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { failure, invalidField, ok, sentBody } from "../test-utils";
import { parseCsv, toCsv } from "./csv";
import { autoMap, candidateBody, countryCode, mappingProblems, sourceValue, splitName } from "./mapping";
import { nameKey, runImport } from "./runImport";

afterEach(() => { vi.restoreAllMocks(); });

describe("csv", () => {
  it("reads Excel files: quotes, commas and line breaks inside quotes, BOM, semicolons", () => {
    expect(parseCsv('﻿Name,Notes\r\n"Tan, Wei","said ""hi""\nthen left"\r\n\r\n')).toEqual([
      ["Name", "Notes"],
      ["Tan, Wei", 'said "hi"\nthen left'],
    ]);
    expect(parseCsv("Name;Email\nSiti;siti@example.com\n")).toEqual([["Name", "Email"], ["Siti", "siti@example.com"]]);
  });

  it("writes what it reads", () => {
    const rows = [["a,b", 'say "x"'], ["plain", "two\nlines"]];
    expect(parseCsv(toCsv(rows))).toEqual(rows);
  });
});

describe("mapping", () => {
  it("matches common column names", () => {
    expect(autoMap("candidates", ["Full Name", "E-mail", "Mobile Number", "Nationality", "Current Position", "Source", "Shoe size"]))
      .toEqual(["fullName", "email", "phone", "nationality", "currentTitle", "source", ""]);
    expect(autoMap("contacts", ["Company Name", "First Name", "Last Name", "Designation", "Email"]))
      .toEqual(["company", "firstName", "lastName", "title", "email"]);
  });

  it("asks for the columns it needs", () => {
    expect(mappingProblems("candidates", ["email"])).toEqual(["Choose the column with the first name or the full name."]);
    expect(mappingProblems("contacts", ["fullName"])).toEqual(["Choose the column with company name."]);
    expect(mappingProblems("companies", ["name"])).toEqual([]);
  });

  it("tidies values whose meaning is clear", () => {
    expect(countryCode("Malaysia")).toBe("MY");
    expect(countryCode("malaysian")).toBe("MY");
    expect(countryCode("sg")).toBe("SG");
    expect(countryCode("Atlantis")).toBe("Atlantis"); // left for the API to refuse
    expect(sourceValue("Job Street")).toBe("jobstreet");
    expect(sourceValue("Walk-in")).toBe("direct");
    expect(sourceValue("Facebook")).toBe("other");
    expect(splitName("Siti Aisyah binti Ali")).toEqual(["Siti", "Aisyah binti Ali"]);
    expect(candidateBody({ fullName: "Wei Tan", nationality: "Singaporean" })).toMatchObject({ firstName: "Wei", lastName: "Tan", nationality: "SG", email: null });
    expect(nameKey("  Nestlé   MALAYSIA ")).toBe("nestle malaysia");
  });
});

describe("running an import", () => {
  it("reports each row: added, already there (with a link), or what to fix", async () => {
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(ok({ id: "cand_1" }))
      .mockResolvedValueOnce(failure("CANDIDATE_DUPLICATE_EMAIL", "Another candidate already has this email.", { existingCandidateId: "cand_9" }, 409))
      .mockResolvedValueOnce(invalidField("phone", "is not a valid phone number"));
    const rows = [["Siti", "siti@example.com", ""], ["Siti Again", "siti@example.com", ""], ["Wei", "", "12"]];
    const progress = vi.fn();
    const results = await runImport("candidates", rows, ["fullName", "email", "phone"], { createCompanies: false }, progress);
    expect(results.map((r) => [r.line, r.status])).toEqual([[2, "added"], [3, "exists"], [4, "fix"]]);
    expect(results[1]?.link).toBe("#/candidates/cand_9");
    expect(results[2]?.message).toBe("phone: is not a valid phone number");
    expect(progress).toHaveBeenLastCalledWith(3);
    expect(sentBody(post)).toMatchObject({ firstName: "Siti", email: "siti@example.com", phone: null });
  });

  it("finds contacts' companies by name, adding missing ones once", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ id: "comp_a", name: "Nestlé Malaysia" }], nextCursor: null }));
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) =>
      Promise.resolve(ok(path === "/v1/companies" ? { id: "comp_new" } : { id: "cont_x" }))) as never);
    const rows = [["nestle malaysia", "Aina"], ["Grab", "Wei"], ["Grab", "Mei"]];
    const results = await runImport("contacts", rows, ["company", "fullName"], { createCompanies: true }, vi.fn());
    expect(results.every((r) => r.status === "added")).toBe(true);
    const calls: [string, unknown][] = post.mock.calls.map((c) => [String(c[0]), (c[1] as { body: unknown }).body]);
    expect(calls.filter(([path]) => path === "/v1/companies")).toHaveLength(1);
    expect(calls).toContainEqual(["/v1/contacts", expect.objectContaining({ companyId: "comp_a", firstName: "Aina" })]);
    expect(calls).toContainEqual(["/v1/contacts", expect.objectContaining({ companyId: "comp_new", firstName: "Mei" })]);
  });
});
