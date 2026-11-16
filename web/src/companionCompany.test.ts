// @vitest-environment jsdom
// @vitest-environment-options {"url": "https://www.linkedin.com/company/maju-logistik/about/"}
// The LinkedIn companion's company.js (extension/linkedin-companion): what a
// company's About page says of it, for a new company in HiringPlug.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { beforeAll, describe, expect, it } from "vitest";

interface About { name: string; industry: string; size: string; website: string; headquarters: string; about: string }
declare global { var HiringplugCompany: { readAbout: (url: string) => About | { loading: true } | { failed: true } | { blocked: string } } }

beforeAll(() => {
  // jsdom lays nothing out: every element counts as shown, and innerText is its text.
  HTMLElement.prototype.getClientRects = function () { return [{}] as unknown as DOMRectList; };
  Object.defineProperty(HTMLElement.prototype, "innerText", { get(this: HTMLElement) { return this.textContent; }, configurable: true });
  window.eval(readFileSync(resolve(__dirname, "../../extension/linkedin-companion/company.js"), "utf8"));
});

const page = `<main>
  <section><h1>Maju Logistik Sdn Bhd</h1><p>Truck Transportation · Shah Alam</p></section>
  <section><h2>Overview</h2><p>Maju moves goods across the peninsula,
    with 300 trucks.</p>
    <dl>
      <dt><h3>Website</h3></dt><dd><a href="https://www.linkedin.com/redir/redirect?url=x">https://maju.example.my</a></dd>
      <dt><h3>Industry</h3></dt><dd>Truck Transportation</dd>
      <dt><h3>Company size</h3></dt><dd>51-200 employees</dd><dd>178 associated members</dd>
      <dt><h3>Headquarters</h3></dt><dd>Shah Alam, Selangor</dd>
      <dt><h3>Founded</h3></dt><dd>1998</dd>
    </dl>
  </section></main>`;

/** Why the read stopped, if it did. */
const blocked = (url: string) => (globalThis.HiringplugCompany.readAbout(url) as { blocked?: string }).blocked;

describe("a company's LinkedIn About page", () => {
  it("gives its name, industry, size (not the members line), website, headquarters and overview", () => {
    document.body.innerHTML = page;
    expect(globalThis.HiringplugCompany.readAbout("https://www.linkedin.com/company/maju-logistik/")).toEqual({
      name: "Maju Logistik Sdn Bhd", industry: "Truck Transportation", size: "51-200 employees", website: "https://maju.example.my",
      headquarters: "Shah Alam, Selangor", about: "Maju moves goods across the peninsula, with 300 trucks.",
    });
  });

  it("waits while the page loads, and never reads another company's page", () => {
    document.body.innerHTML = "<main><h1>Maju Logistik Sdn Bhd</h1></main>";
    expect(globalThis.HiringplugCompany.readAbout("https://www.linkedin.com/company/maju-logistik/")).toEqual({ loading: true });
    document.body.innerHTML = page;
    expect(globalThis.HiringplugCompany.readAbout("https://www.linkedin.com/company/another-co/")).toEqual({ failed: true });
  });

  it("stops, as CV Studio's companion does, at a sign-in, a visible check or a restriction, and never reads past it", () => {
    const url = "https://www.linkedin.com/company/maju-logistik/";
    document.body.innerHTML = `<main><h1>Security verification</h1></main>`;
    expect(blocked(url)).toMatch(/restriction/);
    document.body.innerHTML = page.replace("<main>", "<main><iframe src='https://www.linkedin.com/captcha?x=1'></iframe>");
    expect(blocked(url)).toMatch(/verification check/);
    document.body.innerHTML = page.replace("<main>", "<main><iframe src='https://www.google.com/recaptcha/api2/anchor?size=invisible'></iframe>");
    expect(globalThis.HiringplugCompany.readAbout(url), "an invisible captcha is not a check").toMatchObject({ name: "Maju Logistik Sdn Bhd" });
    document.body.innerHTML = page.replace("<main>", "<main><p>We noticed unusual activity on your account.</p>");
    expect(blocked(url)).toMatch(/restriction/);
  });

  it("never reads a page that names itself as another company", () => {
    document.head.innerHTML = '<link rel="canonical" href="https://www.linkedin.com/company/another-co/">';
    document.body.innerHTML = page;
    expect(globalThis.HiringplugCompany.readAbout("https://www.linkedin.com/company/maju-logistik/")).toEqual({ failed: true });
    document.head.innerHTML = "";
  });
});
