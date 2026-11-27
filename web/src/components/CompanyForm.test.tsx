import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Company } from "../api/client";
import { failure, fakeCompanion, ok, renderWithClient, sentBody } from "../test-utils";
import { CompanyForm } from "./CompanyForm";
import { companyPage, sizeFromLinkedin } from "./linkedinCompany";

let stopCompanion = () => {};
afterEach(() => {
  stopCompanion();
  cleanup();
  vi.restoreAllMocks();
});

const company: Company = {
  id: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, name: "Maybank", type: "client", category: null, linkedinUrl: null,
  website: "https://maybank.com", industry: "Financial Services", size: "enterprise", phone: null, address: null,
  city: "Kuala Lumpur", country: "MY", registrationNumber: null, about: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

const change = (label: string, value: string) => {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
};

describe("company form", () => {
  it("adds a company with every field and hands it back", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    const onSaved = vi.fn();
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} onSaved={onSaved} />);
    change("Name", " Maybank ");
    change("Website", "maybank.com");
    change("Size", "enterprise");
    change("Country (2-letter code)", "my");
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    await waitFor(() => { expect(onSaved).toHaveBeenCalledWith(company); });
    expect(sentBody(post)).toMatchObject({ name: "Maybank", type: "client", website: "maybank.com", size: "enterprise", country: "MY", city: null,
      fillFromWebsite: true, nameGuessed: false }); // a typed name stays: the website only suggests
  });

  it("guesses a new company's name from its addresses, and reads its website once saved", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju-foods-berhad");
    expect(screen.getByLabelText<HTMLInputElement>("Name").value).toBe("Maju Foods Berhad");
    change("Website", "majufoods.com.my");
    expect(screen.getByLabelText<HTMLInputElement>("Name").value, "LinkedIn keeps its words apart").toBe("Maju Foods Berhad");
    expect(screen.getByText(/Saving reads the website and fills in what is left empty/).textContent).toContain("the company’s own name");
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ name: "Maju Foods Berhad", website: "majufoods.com.my", fillFromWebsite: true, nameGuessed: true });
  });

  it("stops guessing once the name is typed, and reads nothing without a website", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju");
    change("Name", "Maju Group");
    change("LinkedIn page", "linkedin.com/company/maju-foods");
    expect(screen.getByLabelText<HTMLInputElement>("Name").value).toBe("Maju Group");
    expect(screen.queryByText(/Saving reads the website/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).not.toHaveProperty("fillFromWebsite");
  });

  it("asks for a name before sending", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    expect(await screen.findByText("Enter the company's name.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });

  it("says when the name is taken", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("COMPANY_DUPLICATE_NAME", "Another company already has this name.", { existingCompanyId: company.id }, 409));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("Name", "Maybank");
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Another company already has this name.");
  });

  it("saves an edit with the version it was opened with", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...company, version: 4 }));
    const onDone = vi.fn();
    const { rerender } = renderWithClient(<CompanyForm company={company} onDone={onDone} />);
    rerender(<CompanyForm company={{ ...company, version: 3, phone: "+60 3-2070 8833" }} onDone={onDone} />);
    change("City", "Petaling Jaya");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ version: 2, city: "Petaling Jaya" });
  });

  it("saves over the industry the AI picked meanwhile, keeping both", async () => {
    const fresh = { ...company, industry: null, industrySelections: { catalogueVersion: 1, ids: [] }, industryPending: true };
    const picked = { ...fresh, version: 3, industry: "Financial Services", industrySelections: { catalogueVersion: 1, ids: ["industry_banking"], primaryId: "industry_banking" }, industryPending: false };
    const patch = vi.spyOn(api, "PATCH")
      .mockResolvedValueOnce(failure("CONFLICT_STALE_VERSION", "Changed by someone else.", undefined, 409))
      .mockResolvedValueOnce(ok({ ...picked, version: 4, city: "Petaling Jaya" }));
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/companies/{companyId}" ? ok(picked) : ok({ lists: [], mandatory: [] }))) as never);
    const onDone = vi.fn();
    renderWithClient(<CompanyForm company={fresh} onDone={onDone} />);
    change("City", "Petaling Jaya");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(patch).toHaveBeenCalledTimes(2);
    const retry = sentBody(patch, 1) as Record<string, unknown>;
    expect(retry).toMatchObject({ version: 3, city: "Petaling Jaya" });
    expect(retry.industrySelections, "the AI's pick stays").toBeUndefined();
    expect("industry" in retry).toBe(false);
  });

  it("still says so when someone else changed more than the industry", async () => {
    const fresh = { ...company, industrySelections: { catalogueVersion: 1, ids: [] } };
    vi.spyOn(api, "PATCH").mockResolvedValue(failure("CONFLICT_STALE_VERSION", "Changed by someone else.", undefined, 409));
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/companies/{companyId}" ? ok({ ...fresh, version: 3, phone: "+60 3-1111 2222" }) : ok({ lists: [], mandatory: [] }))) as never);
    renderWithClient(<CompanyForm company={fresh} onDone={vi.fn()} />);
    change("City", "Petaling Jaya");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/Reload their version/)).toBeTruthy();
  });

  it("closes without saving when nothing changed", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(company));
    const onDone = vi.fn();
    renderWithClient(<CompanyForm company={company} onDone={onDone} />);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(patch).not.toHaveBeenCalled();
  });

  it("fills a new company in from its LinkedIn page with the companion, leaving what was typed", async () => {
    const fake = fakeCompanion(() => ({ result: {
      company_url: "https://www.linkedin.com/company/maju-logistik/", name: "Maju Logistik Sdn Bhd", industry: "Truck Transportation",
      size: "51-200 employees", website: "https://maju.example.my", headquarters: "Shah Alam", about: "Trucks across the peninsula.",
    } }));
    stopCompanion = fake.stop;
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(company));
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} onSaved={vi.fn()} />);
    change("LinkedIn page", "my.linkedin.com/company/maju-logistik/about");
    change("City", "Klang");
    fireEvent.click(await screen.findByRole("button", { name: "Fill in from LinkedIn" }));
    expect((await screen.findByRole("status")).textContent).toContain("From LinkedIn: name, size, website, about.");
    expect(fake.asked[0]).toMatchObject({ action: "companyAbout", args: { company_url: "https://www.linkedin.com/company/maju-logistik/" } });
    expect(screen.getByLabelText<HTMLInputElement>("Name").value, "LinkedIn's name replaces the one guessed from the address").toBe("Maju Logistik Sdn Bhd");
    fireEvent.click(screen.getByRole("button", { name: "Add company" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ name: "Maju Logistik Sdn Bhd", size: "medium", website: "https://maju.example.my", city: "Klang",
      industryHint: "Truck Transportation", about: "Trucks across the peninsula.", fillFromWebsite: true, nameGuessed: false });
    expect((sentBody(post) as { industry?: string }).industry, "LinkedIn's industry is a hint, never one of ours").toBeFalsy();
  });

  it("fills the city from LinkedIn's headquarters when it is empty", async () => {
    const fake = fakeCompanion(() => ({ result: {
      company_url: "https://www.linkedin.com/company/maju-logistik/", name: "Maju Logistik Sdn Bhd", industry: "", size: "",
      website: "", headquarters: "Shah Alam, Selangor", about: "",
    } }));
    stopCompanion = fake.stop;
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju-logistik");
    fireEvent.click(await screen.findByRole("button", { name: "Fill in from LinkedIn" }));
    expect((await screen.findByRole("status")).textContent).toContain("From LinkedIn: name, city.");
    expect(screen.getByLabelText<HTMLInputElement>("City").value).toBe("Shah Alam");
  });

  it("says why LinkedIn could not be read, and offers the companion when it is not installed", async () => {
    const fake = fakeCompanion(() => ({ error: "LinkedIn needs a verification check. Nothing was read." }));
    stopCompanion = fake.stop;
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju-logistik");
    fireEvent.click(await screen.findByRole("button", { name: "Fill in from LinkedIn" }));
    expect((await screen.findByRole("alert")).textContent).toContain("verification check");
    fake.stop();
    cleanup();
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju-logistik");
    expect(await screen.findByText(/With the HyPlug LinkedIn companion/, {}, { timeout: 3000 })).toBeTruthy();
  });
});

describe("Fill in from LinkedIn, by CV Studio's companion rules", () => {
  it("stops the companion's own read, and ignores its late answer", async () => {
    const fake = fakeCompanion((action) => (action === "cancel" ? { result: true } : null)); // the read answers late
    stopCompanion = fake.stop;
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju-logistik");
    change("Name", "Typed Co");
    fireEvent.click(await screen.findByRole("button", { name: "Fill in from LinkedIn" }));
    fireEvent.click(await screen.findByRole("button", { name: "Stop" }));
    await waitFor(() => { expect(fake.asked.map((a) => a.action)).toContain("cancel"); });
    const read = fake.asked.find((a) => a.action === "companyAbout");
    expect(fake.asked.find((a) => a.action === "cancel")?.args.operation_id, "this read, not another").toBe(read?.args.operation_id);
    fake.reply(read?.id ?? "", { result: { company_url: "https://www.linkedin.com/company/maju-logistik/", name: "Maju Logistik Sdn Bhd",
      industry: "", size: "51-200 employees", website: "", headquarters: "", about: "" } });
    await new Promise((r) => { setTimeout(r, 20); });
    expect(screen.getByLabelText<HTMLInputElement>("Name").value, "nothing filled after Stop").toBe("Typed Co");
    expect(screen.getByLabelText<HTMLSelectElement>("Size").value).toBe("");
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("asks for an update when the companion is older than company reading", async () => {
    const fake = fakeCompanion(() => ({ result: null }), "1.0.0");
    stopCompanion = fake.stop;
    renderWithClient(<CompanyForm company={null} onDone={vi.fn()} />);
    change("LinkedIn page", "linkedin.com/company/maju-logistik");
    expect(await screen.findByText(/Update the HyPlug LinkedIn companion/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Fill in from LinkedIn" })).toBeNull();
  });
});

describe("LinkedIn company sizes", () => {
  it("are put in our size bands by their lower figure", () => {
    expect(["1 employee", "2-10 employees", "11-50 employees", "51-200 employees", "501-1,000 employees", "1,001-5,000 employees", "10,001+ employees", "big"]
      .map(sizeFromLinkedin)).toEqual(["micro", "micro", "small", "medium", "large", "enterprise", "enterprise", null]);
    expect(companyPage("my.linkedin.com/company/maju-logistik/about")).toBe("https://www.linkedin.com/company/maju-logistik/");
    expect(companyPage("linkedin.com/in/someone")).toBe("");
  });
});
