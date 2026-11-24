import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { templateKeys, unfilledKeys, useInsertAtCursor, valuesLeft, valuesToFields, valueToField } from "./whatsAppText";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

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

describe("valuesToFields: names and ordinary words", () => {
  it("gives the first name a name that is also the whole full name (no last name)", () => {
    const siti = [{ key: "candidate.firstName", value: "Siti" }, { key: "candidate.lastName", value: "" }, { key: "candidate.fullName", value: "Siti" }];
    expect(valuesToFields("Hi Siti, hello Siti", siti)).toBe("Hi {{candidate.firstName}}, hello {{candidate.firstName}}");
    // A contact with no last name: the client contact's name is theirs too.
    const contact = [...siti.map((f) => ({ ...f, value: f.value && "Aisyah" })), { key: "client.name", value: "Aisyah" }];
    expect(valuesToFields("Dear Aisyah", contact)).toBe("Dear {{candidate.firstName}}");
  });

  it("leaves a one-word value that starts a sentence as an ordinary word, unless a comma follows or it is alone on its line", () => {
    const may = [{ key: "candidate.firstName", value: "May" }, { key: "sender.name", value: "Will" }];
    expect(valuesToFields("May I call you? Hi May, thanks. Will do. Will you reply?", may))
      .toBe("May I call you? Hi {{candidate.firstName}}, thanks. Will do. Will you reply?");
    expect(valuesToFields("May, are you free?\nThanks!\nWill", may)).toBe("{{candidate.firstName}}, are you free?\nThanks!\n{{sender.name}}");
    expect(valuesToFields("Hope you are well", [{ key: "candidate.firstName", value: "Hope" }])).toBe("Hope you are well");
  });

  it("turns back the fields the source template used, after the usual ones", () => {
    const fields = [{ key: "candidate.nationality", value: "Malaysian" }, { key: "candidate.currentCompany", value: "Contoh Berhad" }, { key: "company.name", value: "Contoh Berhad" }];
    expect(valuesToFields("A Malaysian at Contoh Berhad", fields)).toBe("A Malaysian at {{company.name}}");
    expect(valuesToFields("A Malaysian at Contoh Berhad", fields, ["candidate.nationality", "candidate.currentCompany"]))
      .toBe("A {{candidate.nationality}} at {{company.name}}");
  });
});

describe("the fields a template uses, left unfilled, or still written as values", () => {
  it("lists a template's fields", () => {
    expect(templateKeys("Hi {{candidate.firstName}}, {{ job.title }}?")).toEqual(["candidate.firstName", "job.title"]);
  });

  it("lists the [key] fields still in a message once each, in order", () => {
    expect(unfilledKeys("A [job.title] at [company.name], [job.title] [see note]", ["company.name", "job.title"])).toEqual(["job.title", "company.name"]);
    expect(unfilledKeys("All filled", ["job.title"])).toEqual([]);
  });

  it("finds values still in a template's text (not inside fields) and replaces one with its field", () => {
    const fields = [
      { key: "candidate.nationality", label: "Nationality", value: "Malaysian" },
      { key: "candidate.firstName", label: "First name", value: "Siti" },
      { key: "candidate.residence", label: "Residence", value: "" },
    ];
    expect(valuesLeft("{{candidate.firstName}}, a Malaysian role for Sitiawan", fields).map((f) => f.key)).toEqual(["candidate.nationality"]);
    expect(valueToField("Malaysian, {{x.Malaysian}} Malaysians Malaysian", "Malaysian", "candidate.nationality"))
      .toBe("{{candidate.nationality}}, {{x.Malaysian}} Malaysians {{candidate.nationality}}");
    // A first name that is also an ordinary word: not pointed out, and never replaced, where it starts a sentence.
    const may = [{ key: "candidate.firstName", label: "First name", value: "May" }];
    expect(valuesLeft("May I call you tomorrow?", may)).toEqual([]);
    expect(valuesLeft("May I call you? Hi May, thanks", may).map((f) => f.key)).toEqual(["candidate.firstName"]);
    expect(valueToField("May I call you? Hi May, thanks", "May", "candidate.firstName")).toBe("May I call you? Hi {{candidate.firstName}}, thanks");
  });
});

describe("useInsertAtCursor", () => {
  it("listens for the box's focus once, not again on every render, and still puts text at the cursor", () => {
    const node = document.body.appendChild(document.createElement("textarea"));
    const box = { current: node };
    const listening = vi.spyOn(EventTarget.prototype, "addEventListener");
    const focusListeners = () => listening.mock.calls.filter(([type]) => type === "focus" || type === "focusin").length;
    const onChange = vi.fn();
    const { result, rerender } = renderHook(({ text }) => useInsertAtCursor(box, text, onChange), { initialProps: { text: "Hi , welcome" } });
    const once = focusListeners();
    for (const text of ["Hi , welcome!", "Hi , welcome!!", "Hi , welcome"]) rerender({ text });
    expect(focusListeners()).toBe(once);

    node.value = "Hi , welcome";
    node.focus();
    node.setSelectionRange(3, 3);
    node.blur(); // to the Insert field list
    result.current("Siti");
    expect(onChange).toHaveBeenLastCalledWith("Hi Siti, welcome");
    node.remove();
  });
});
