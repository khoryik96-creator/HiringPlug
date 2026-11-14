import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { careersRoute, careersUrl } from "../careersRoute";
import { Careers } from "./Careers";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const ad = {
  id: "ad_01jbz6w0m0k8q9x2r3s4t5v6w7", title: "Senior Accountant", location: "Kuala Lumpur", workType: "full_time" as const,
  workArrangement: null, salaryText: "RM 8,000", description: "Join us.\n- Month-end close", closesOn: null,
  publishedAt: "2026-11-01T00:00:00Z",
};
const page = () => vi.spyOn(api, "GET").mockResolvedValue(ok({ companyName: "Contoh Berhad", ads: [ad] }));

describe("careers page", () => {
  it("lists open jobs with links to each", async () => {
    page();
    renderWithClient(<Careers route={{ slug: "contoh-berhad", adId: undefined, via: undefined }} pathStyle={false} />);
    const link = await screen.findByRole("link", { name: "Senior Accountant" });
    expect(link.getAttribute("href")).toBe(`#/careers/contoh-berhad/${ad.id}`);
    expect(screen.getByText("Kuala Lumpur · Full time · RM 8,000")).toBeTruthy();
  });

  it("sends the application with the CV, consent and the empty robot trap, then thanks", async () => {
    page();
    const send = vi.fn().mockResolvedValue(new Response(JSON.stringify({ received: true }), { status: 202 }));
    vi.stubGlobal("fetch", send);
    renderWithClient(<Careers route={{ slug: "contoh-berhad", adId: ad.id, via: undefined }} pathStyle={false} />);
    await screen.findByRole("heading", { name: "Senior Accountant" });
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Nur" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "nur@example.com" } });
    const submit = screen.getByRole<HTMLButtonElement>("button", { name: "Send application" });
    fireEvent.click(submit); // a CV and consent first
    expect(send).not.toHaveBeenCalled();
    expect(screen.getByLabelText<HTMLInputElement>(/I agree that Contoh Berhad keeps my details/).validity.valueMissing).toBe(true);
    fireEvent.change(screen.getByLabelText("CV (PDF, Word, text or a photo)"), { target: { files: [new File(["%PDF"], "cv.pdf")] } });
    fireEvent.click(screen.getByLabelText(/I agree that Contoh Berhad keeps my details/));
    fireEvent.click(submit);
    expect(await screen.findByRole("status")).toBeTruthy();
    const [url, init] = send.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`/v1/public/careers/contoh-berhad/ads/${ad.id}/applications`);
    const body = init.body as FormData;
    expect([body.get("firstName"), body.get("email"), body.get("consent"), body.get("website")]).toEqual(["Nur", "nur@example.com", "yes", ""]);
    expect((body.get("file") as File).name).toBe("cv.pdf");
    expect(new Headers(init.headers).get("Authorization")).toBeNull(); // never the staff token
  });

  it("shows what to fix when the form is refused", async () => {
    page();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "VALIDATION_FAILED", message: "Some fields are invalid.", requestId: "req_1", details: { fields: [{ field: "email", message: "is not a valid email address" }] } },
    }), { status: 400 })));
    renderWithClient(<Careers route={{ slug: "contoh-berhad", adId: ad.id, via: undefined }} pathStyle={false} />);
    await screen.findByRole("heading", { name: "Senior Accountant" });
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Nur" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "nur@localhost" } });
    fireEvent.change(screen.getByLabelText("CV (PDF, Word, text or a photo)"), { target: { files: [new File(["%PDF"], "cv.pdf")] } });
    fireEvent.click(screen.getByLabelText(/I agree/));
    fireEvent.click(screen.getByRole("button", { name: "Send application" }));
    await waitFor(() => { expect(screen.getByText("email: is not a valid email address")).toBeTruthy(); });
  });

  it("says when a job is no longer open", async () => {
    page();
    renderWithClient(<Careers route={{ slug: "contoh-berhad", adId: "ad_01jbz6w0m0k8q9x2r3s4t5zzzz", via: undefined }} pathStyle={false} />);
    expect(await screen.findByText(/This job is no longer open/)).toBeTruthy();
  });
});

describe("job board links", () => {
  it("reads the page, the ad and the board from real and # addresses", () => {
    expect(careersRoute({ pathname: "/careers/contoh-berhad/ad_01jbz6w0m0k8q9x2r3s4t5v6w7", search: "?via=linkedin", hash: "" }))
      .toEqual({ slug: "contoh-berhad", adId: "ad_01jbz6w0m0k8q9x2r3s4t5v6w7", via: "linkedin" });
    expect(careersRoute({ pathname: "/", search: "", hash: "#/careers/contoh-berhad?via=jobstreet" }))
      .toEqual({ slug: "contoh-berhad", adId: undefined, via: "jobstreet" });
    expect(careersRoute({ pathname: "/", search: "", hash: "#/candidates" })).toBeNull();
    expect(careersUrl("https://hire.example", "contoh-berhad", ad.id, "indeed"))
      .toBe(`https://hire.example/careers/contoh-berhad/${ad.id}?via=indeed`);
  });

  it("keeps the board while browsing and sends it with the application", async () => {
    page();
    const send = vi.fn().mockResolvedValue(new Response(JSON.stringify({ received: true }), { status: 202 }));
    vi.stubGlobal("fetch", send);
    renderWithClient(<Careers route={{ slug: "contoh-berhad", adId: ad.id, via: "linkedin" }} pathStyle />);
    expect((await screen.findByRole("link", { name: "← All jobs" })).getAttribute("href")).toBe("/careers/contoh-berhad?via=linkedin");
    fireEvent.change(screen.getByLabelText("First name"), { target: { value: "Nur" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "nur@example.com" } });
    fireEvent.change(screen.getByLabelText("CV (PDF, Word, text or a photo)"), { target: { files: [new File(["%PDF"], "cv.pdf")] } });
    fireEvent.click(screen.getByLabelText(/I agree/));
    fireEvent.click(screen.getByRole("button", { name: "Send application" }));
    await screen.findByRole("status");
    expect(((send.mock.calls[0] as [string, RequestInit])[1].body as FormData).get("via")).toBe("linkedin");
  });

  it("describes the ad for Google for Jobs while it is shown", async () => {
    page();
    renderWithClient(<Careers route={{ slug: "contoh-berhad", adId: ad.id, via: undefined }} pathStyle />);
    await screen.findByRole("heading", { name: "Senior Accountant" });
    const script = document.head.querySelector('script[type="application/ld+json"]');
    expect(JSON.parse(script?.textContent ?? "{}")).toMatchObject({
      "@type": "JobPosting", title: "Senior Accountant", employmentType: "FULL_TIME", datePosted: "2026-11-01",
      hiringOrganization: { name: "Contoh Berhad" },
    });
    cleanup(); // the visitor leaves the ad
    expect(document.head.querySelector('script[type="application/ld+json"]')).toBeNull();
  });
});
