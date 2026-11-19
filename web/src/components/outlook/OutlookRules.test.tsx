import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { failure, ok, renderWithClient, sentBody } from "../../test-utils";
import { OutlookRules } from "./OutlookRules";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("offers an explicit permission upgrade when the mailbox cannot access rules", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(failure("OUTLOOK_RULES_ACCESS", "Outlook needs rules permission.", undefined, 403));
  const post = vi.spyOn(api, "POST");
  renderWithClient(<OutlookRules />);
  expect(await screen.findByRole("button", { name: "Allow inbox rules" })).toBeTruthy();
  expect(post).not.toHaveBeenCalled();
});

it("offers the permission upgrade when a mailbox can list rules but cannot change them", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({
    items: [{ id: "rule", name: "Applications", enabled: true, readOnly: false, hasError: false, destination: "Archive" }],
    folders: [],
  }));
  vi.spyOn(api, "PATCH").mockResolvedValue(failure("OUTLOOK_RULES_ACCESS", "Outlook needs rules permission.", undefined, 403));
  renderWithClient(<OutlookRules />);
  fireEvent.click(await screen.findByRole("button", { name: "Turn off Applications" }));
  expect(await screen.findByRole("button", { name: "Allow inbox rules" })).toBeTruthy();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Turn off Applications" }).disabled).toBe(true);
});

it("creates a rule with the chosen sender, subject and Outlook folder", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], folders: [{ id: "omf_archive", name: "Archive", kind: "archive" }] }));
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "rule", name: "Applications" }));
  renderWithClient(<OutlookRules />);
  expect((await screen.findByRole<HTMLButtonElement>("button", { name: "Create rule" })).disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Rule name"), { target: { value: "Applications" } });
  fireEvent.change(screen.getByLabelText("Move to"), { target: { value: "omf_archive" } });
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create rule" }).disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Sender address"), { target: { value: "alerts@example.test" } });
  fireEvent.change(screen.getByLabelText("Subject contains"), { target: { value: "New applicant" } });
  fireEvent.click(screen.getByRole("button", { name: "Create rule" }));
  await waitFor(() => { expect(post).toHaveBeenCalled(); });
  expect(sentBody(post)).toEqual({
    name: "Applications", folderId: "omf_archive", fromAddress: "alerts@example.test", subjectContains: "New applicant",
  });
});
