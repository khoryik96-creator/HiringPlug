import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type ContactSummary } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { ContactList } from "./ContactList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const contact = (over: Partial<ContactSummary>): ContactSummary => ({
  id: "cont_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Mei Tan", title: "HR Manager", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7",
  companyName: "Maju", email: null, emailCheckStatus: null, phone: null, mobile: null, status: "active", role: "hr",
  updatedAt: "2026-11-01T00:00:00Z", ...over,
});

it("shows each contact's role and whether they still work there, and filters by both", async () => {
  const asked: Record<string, unknown>[] = [];
  vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: Record<string, unknown> } }) => {
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    asked.push(init?.params?.query ?? {});
    return Promise.resolve(ok({ nextCursor: null, items: [
      contact({}),
      contact({ id: "cont_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Ali Hassan", role: null, status: "left" }),
    ] }));
  }) as never);
  renderWithClient(<ContactList canCreate={false} />);
  const mei = (await screen.findByRole("link", { name: "Mei Tan" })).closest("tr");
  const ali = screen.getByRole("link", { name: "Ali Hassan" }).closest("tr");
  if (!mei || !ali) throw new Error("rows");
  await waitFor(() => { expect(within(mei).getByText("HR").getAttribute("style")).toContain("border-color: rgb(219, 39, 119)"); });
  expect(within(mei).getByText("Works there")).toBeTruthy();
  expect(within(ali).getByText("Not set")).toBeTruthy();
  expect(within(ali).getByText("Has left")).toBeTruthy();

  await screen.findByRole("option", { name: "Director" });
  fireEvent.change(screen.getByLabelText("Role"), { target: { value: "hiring_manager" } });
  await waitFor(() => { expect(asked.at(-1)).toEqual({ limit: 100, role: "hiring_manager" }); });
  fireEvent.change(screen.getByLabelText("Status"), { target: { value: "left" } });
  await waitFor(() => { expect(asked.at(-1)).toEqual({ limit: 100, role: "hiring_manager", status: "left" }); });
  fireEvent.change(screen.getByLabelText("Role"), { target: { value: "-" } });
  await waitFor(() => { expect(asked.at(-1)).toEqual({ limit: 100, roleNotSet: true, status: "left" }); });
});
