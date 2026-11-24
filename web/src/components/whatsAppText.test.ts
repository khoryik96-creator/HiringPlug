import { describe, expect, it } from "vitest";

import { valuesToFields } from "./whatsAppText";

const fields = [
  { key: "candidate.firstName", value: "Ali" },
  { key: "candidate.lastName", value: "" },
  { key: "candidate.fullName", value: "Ali Hassan" },
  { key: "job.title", value: "Finance Manager (KL)" },
  { key: "company.name", value: "Contoh Berhad" },
  { key: "client.name", value: "Ali Hassan" },
  { key: "sender.name", value: "S" },
];

describe("valuesToFields", () => {
  it("turns the person's values back into fields, longer values first", () => {
    expect(valuesToFields("Hi Ali Hassan, Ali here? The Finance Manager (KL) role at Contoh Berhad.", fields))
      .toBe("Hi {{candidate.fullName}}, {{candidate.firstName}} here? The {{job.title}} role at {{company.name}}.");
  });

  it("matches whole words only, so a value inside a longer word stays", () => {
    expect(valuesToFields("Alif and Ali; Khalid; Ali's", fields)).toBe("Alif and {{candidate.firstName}}; Khalid; {{candidate.firstName}}'s");
  });

  it("leaves empty and one-letter values alone, and never matches inside a field already put in", () => {
    expect(valuesToFields("S. at Contoh Berhad", fields)).toBe("S. at {{company.name}}");
    expect(valuesToFields("Contoh Berhad, a company", [{ key: "company.name", value: "Contoh Berhad" }, { key: "agency.name", value: "company" }]))
      .toBe("{{company.name}}, a {{agency.name}}");
  });

  it("gives a shared value to the person's name rather than the client contact's", () => {
    expect(valuesToFields("Dear Ali Hassan", fields)).toBe("Dear {{candidate.fullName}}");
  });

  it("gives a value shared with the candidate's current title or company to the job and client company", () => {
    const pitched = [
      { key: "candidate.firstName", value: "Siti" },
      { key: "candidate.currentTitle", value: "Finance Manager" },
      { key: "candidate.currentCompany", value: "Contoh Berhad" },
      { key: "job.title", value: "Finance Manager" },
      { key: "company.name", value: "Contoh Berhad" },
    ];
    expect(valuesToFields("Siti, I have a Finance Manager role at Contoh Berhad", pitched))
      .toBe("{{candidate.firstName}}, I have a {{job.title}} role at {{company.name}}");
  });

  it("leaves the candidate's place, industry, nationality, notice period and current job as written", () => {
    const profile = [
      { key: "candidate.residence", value: "Kuala Lumpur" },
      { key: "candidate.industries", value: "Finance" },
      { key: "candidate.nationality", value: "Malaysian" },
      { key: "candidate.noticePeriod", value: "1 month" },
      { key: "candidate.currentTitle", value: "Audit Senior" },
      { key: "candidate.currentCompany", value: "Syarikat Contoh" },
    ];
    const text = "A Finance role at a Malaysian company in Kuala Lumpur, a 1 month trial; Audit Senior at Syarikat Contoh suits.";
    expect(valuesToFields(text, profile)).toBe(text);
  });

  it("turns a field that had no value ([job.title]) back into a field, and leaves other brackets alone", () => {
    const unfilled = [{ key: "candidate.firstName", value: "Siti" }, { key: "job.title", value: "" }, { key: "company.name", value: "" }];
    expect(valuesToFields("Hi Siti, I have a [job.title] role at [company.name] [see note] [candidate.fullName]", unfilled))
      .toBe("Hi {{candidate.firstName}}, I have a {{job.title}} role at {{company.name}} [see note] [candidate.fullName]");
  });

  it("leaves fields already written as {{...}} as they are", () => {
    expect(valuesToFields("{{job.title}} for Ali", [{ key: "job.title", value: "job" }, { key: "candidate.firstName", value: "Ali" }]))
      .toBe("{{job.title}} for {{candidate.firstName}}");
  });

  it("returns the text as it was when nothing matches", () => {
    expect(valuesToFields("Hello there", [])).toBe("Hello there");
  });
});
