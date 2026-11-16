import { describe, expect, it } from "vitest";

import { type Held, maskHtml, maskText, unmask, unmaskSubject } from "./mask";

describe("what the AI sees of an email", () => {
  it("holds back pay, contact details, links and fields, and puts them back as they were", () => {
    const held: Held = new Map();
    const html = '<p>Hi Siti Aisyah,</p><p>The offer is RM 9,500 a month (12,000 with bonus). Call me on +60 12-345 6789 or nurul@hyppies.my.</p>'
      + '<p>Book here: <a href="https://ats.example/book/abc">pick a time</a>. Notice: <span data-field="candidate.noticePeriod">1 month</span>.</p><p>Nurul</p>';
    const text = maskHtml(html, held, { them: "Siti Aisyah", me: "Nurul Huda" });
    for (const leak of ["Siti", "Aisyah", "9,500", "12,000", "345 6789", "nurul@hyppies.my", "ats.example", "1 month", "Nurul"]) {
      expect(text, leak).not.toContain(leak);
    }
    expect(text).toContain("Hi ⟦NAME⟧,");
    expect(text).toContain("\n\n"); // paragraphs kept apart
    expect(text.trim().endsWith("⟦ME⟧")).toBe(true);

    const written = text.replace("Hi ⟦NAME⟧,", "Dear ⟦NAME⟧,");
    const back = unmask(written, held, { them: "Siti Aisyah", me: "Nurul Huda" });
    expect(back).toContain("<p>Dear Siti,</p>");
    expect(back).toContain("RM 9,500");
    expect(back).toContain("12,000");
    expect(back).toContain("+60 12-345 6789");
    expect(back).toContain('<a href="https://ats.example/book/abc">pick a time</a>');
    expect(back).toContain('<span data-field="candidate.noticePeriod">1 month</span>');
    expect(back).toContain("<p>Nurul Huda</p>");
  });

  it("keeps years, and gives the same thing the same placeholder", () => {
    const held: Held = new Map();
    const text = maskText("Since 2019, earning 8k; 8k is what she wants.", held);
    expect(text).toBe("Since 2019, earning ⟦1⟧; ⟦1⟧ is what she wants.");
  });

  it("escapes what the AI wrote, and leaves an unknown placeholder visible", () => {
    const held: Held = new Map();
    expect(unmask("Hi ⟦NAME⟧ <b>bold</b>\n\n⟦7⟧", held, {})).toBe("<p>Hi there &lt;b&gt;bold&lt;/b&gt;</p><p>⟦7⟧</p>");
    held.set("⟦1⟧", "RM 5,000");
    expect(unmaskSubject("Offer: ⟦1⟧ for ⟦NAME⟧", held, { them: "Wei Tan" })).toBe("Offer: RM 5,000 for Wei");
  });
});
