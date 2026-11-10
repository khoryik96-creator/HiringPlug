import { describe, expect, it } from "vitest";

import type { Schemas } from "../../api/client";
import { blocksHtml, DEFAULT_DETAILS, DETAILS, missingDetails, profilesOf, tableHtml } from "./shortlistHtml";

const chosen = DETAILS.filter((d) => DEFAULT_DETAILS.includes(d.id));
const farid = { name: "Farid Hassan", values: { "[Current title]": "Hub Supervisor", "[Current salary]": "RM 7,500 per month", "[Expected salary]": "RM 9,000 per month", "[Notice period]": "2 months", "[Lives in]": "Klang" } };
const nurul = { name: "Nurul Huda", values: { "[Notice period]": "1 month" } }; // not much on record (or salaries the sender may not see)

describe("candidates in an email to the client", () => {
  it("are a block each: their own values, marked as their fields; what is not on record is a dash", () => {
    const html = blocksHtml([farid, nurul], chosen);
    expect(html).toContain("<b>1. Farid Hassan, Hub Supervisor</b>");
    expect(html).toContain(`Expected salary: <span data-field="candidate.expectedSalary">RM 9,000 per month</span>`);
    expect(html).toContain(`Location: <span data-field="candidate.residence">Klang</span>`);
    expect(html).toContain("<b>2. Nurul Huda</b><br>Current salary: — · Expected salary: — · Notice period: <span");
    expect(missingDetails([farid, nurul], chosen)).toEqual(["Nurul Huda: Current salary, Expected salary, Location"]);
  });

  it("or one table, a row each and a column per detail", () => {
    const html = tableHtml([farid, nurul], chosen);
    const doc = new DOMParser().parseFromString(html, "text/html");
    expect([...doc.querySelectorAll("th")].map((th) => th.textContent)).toEqual(["Candidate", "Current salary", "Expected salary", "Notice period", "Location"]);
    const rows = [...doc.querySelectorAll("tbody tr")].map((tr) => [...tr.querySelectorAll("td")].map((td) => td.textContent));
    expect(rows).toEqual([
      ["Farid HassanHub Supervisor", "RM 7,500 per month", "RM 9,000 per month", "2 months", "Klang"],
      ["Nurul Huda", "—", "—", "1 month", "—"],
    ]);
    expect(doc.querySelector("td")?.getAttribute("style")).toContain("border:1px solid");
  });

  it("never let a name or value write HTML", () => {
    const html = blocksHtml([{ name: "<img src=x onerror=alert(1)>", values: { "[Notice period]": "<b>now</b>" } }], chosen);
    expect(html).not.toContain("<img");
    expect(html).toContain("&lt;b&gt;now&lt;/b&gt;");
  });

  it("attach the newest formatted profile (as PDF once made) by default, then the others", () => {
    const doc = (id: string, source: Schemas["DocumentSource"], fileName: string, createdAt: string, pdfStatus: Schemas["PdfStatus"] | null = null) =>
      ({ id, source, fileName, createdAt, pdfStatus }) as Schemas["Document"];
    const profiles = profilesOf([
      doc("d1", "original", "Nurul Huda CV.pdf", "2026-11-01T00:00:00Z"),
      doc("d2", "formatted", "Hyppies CV - Nurul Huda.docx", "2026-11-05T00:00:00Z", "ready"),
      doc("d3", "formatted", "Hyppies CV - Nurul Huda (old).docx", "2026-11-02T00:00:00Z", "failed"),
    ]);
    expect(profiles.map((p) => p.label)).toEqual([
      "Hyppies CV - Nurul Huda (PDF)", "Hyppies CV - Nurul Huda.docx (Word)", "Hyppies CV - Nurul Huda (old).docx (Word)", "Original: Nurul Huda CV.pdf",
    ]);
    expect(profiles[0]?.choice).toEqual({ documentId: "d2", asPdf: true });
  });
});
