// @vitest-environment jsdom
// @vitest-environment-options {"url": "https://www.linkedin.com/in/ana-lim-example/details/experience/"}
// The LinkedIn companion's roles.js (extension/linkedin-companion): where a
// contact who left works now, from their Experience page.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { beforeAll, describe, expect, it } from "vitest";

interface Role { company_url: string; company_name: string; title: string; since: string }
declare global { var HiringplugRoles: { readCurrentRoles: (url: string, left: string) => Role[] } }

beforeAll(() => {
  // jsdom lays nothing out: every element counts as shown, and innerText is its text.
  HTMLElement.prototype.getClientRects = function () { return [{}] as unknown as DOMRectList; };
  Object.defineProperty(HTMLElement.prototype, "innerText", { get(this: HTMLElement) { return this.textContent; }, configurable: true });
  window.eval(readFileSync(resolve(__dirname, "../../extension/linkedin-companion/roles.js"), "utf8"));
});

const card = (company: string, slug: string, lines: string[]) => `
  <li componentkey="entity-collection-item-${slug}">
    <a href="https://www.linkedin.com/company/${slug}/"><img alt="${company} logo"></a>
    ${lines.map((l) => `<p>${l}</p>`).join("")}
  </li>`;

describe("where a contact who left works now", () => {
  it("is the roles shown as current, with each company's page, as precise as shown", () => {
    document.body.innerHTML = `<main><section componentkey="profileExperienceDetailsSection">
      <ul>
        ${card("Acme Bhd", "acme-bhd", ["Head of HR", "Acme Bhd · Full-time", "Sep 2025 - Present · 3 mos", "Kuala Lumpur"])}
        ${card("Volunteer Org", "volunteer-org", ["Mentor", "Volunteer Org", "2024 - Present"])}
        ${card("Maju Sdn Bhd", "maju-logistik", ["HR Manager", "Maju Sdn Bhd · Full-time", "Apr 2019 - Aug 2025 · 6 yrs"])}
        ${card("Old Co", "old-co", ["Clerk", "Old Co", "Jan 2015 - Mar 2019"])}
      </ul></section></main>`;
    const roles = globalThis.HiringplugRoles.readCurrentRoles("https://www.linkedin.com/in/ana-lim-example", "https://www.linkedin.com/company/maju-logistik/");
    expect(roles).toEqual([
      { company_url: "https://www.linkedin.com/company/acme-bhd/", company_name: "Acme Bhd", title: "Head of HR", since: "2025-09" },
      { company_url: "https://www.linkedin.com/company/volunteer-org/", company_name: "Volunteer Org", title: "Mentor", since: "2024" },
    ]);
  });

  it("is nothing on another profile's page, or without the Experience section", () => {
    document.body.innerHTML = `<main><section componentkey="profileExperienceDetailsSection"><ul>
      ${card("Acme Bhd", "acme-bhd", ["Head of HR", "Sep 2025 - Present"])}</ul></section></main>`;
    expect(() => globalThis.HiringplugRoles.readCurrentRoles("https://www.linkedin.com/in/someone-else", "")).toThrow();
    document.body.innerHTML = "<main><p>Sep 2025 - Present</p></main>";
    expect(globalThis.HiringplugRoles.readCurrentRoles("https://www.linkedin.com/in/ana-lim-example", "")).toEqual([]);
  });
});
