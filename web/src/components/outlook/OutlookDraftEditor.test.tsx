import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { ok, renderWithClient, sentBody } from "../../test-utils";
import { zonedInput } from "../calendar/dates";
import { OutlookDraftEditor } from "./OutlookDraftEditor";
import { OutlookScheduledList } from "./OutlookScheduledList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const schedule: Schemas["OutlookScheduledSend"] = {
  id: "oms_test", messageId: "omg_test", subject: "Tuesday interview", status: "scheduled",
  version: 2, sendAt: "2026-11-20T01:00:00Z", updatedAt: "2026-11-19T00:00:00Z", error: null,
};

it("submits the revised draft and new time in the same request", async () => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
  const draft: Schemas["OutlookDraft"] = {
    messageId: "omg_test", version: "opened", subject: "Interview", body: "Tuesday is available.",
    to: ["nadia@example.test"], cc: [], bcc: [], hasAttachments: true, bodyTruncated: false,
    schedule, scheduleNeedsReview: false, webUrl: null,
  };
  vi.spyOn(api, "GET").mockResolvedValue(ok(draft));
  const patch = vi.spyOn(api, "PATCH");
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(schedule));
  const onClose = vi.fn();
  renderWithClient(<OutlookDraftEditor id="omg_test" onClose={onClose}/>);
  fireEvent.change(await screen.findByLabelText("Subject"), { target: { value: "Tuesday interview" } });
  fireEvent.click(screen.getByRole("button", { name: "Change time" }));
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const future = new Date(Math.ceil(Date.now() / 60000) * 60000 + 7200000);
  fireEvent.change(screen.getByLabelText(/Send at/), { target: { value: zonedInput(future, zone) } });
  fireEvent.click(screen.getByRole("button", { name: "Update schedule" }));
  await waitFor(() => { expect(onClose).toHaveBeenCalledWith("schedule"); });
  expect(patch).not.toHaveBeenCalled();
  expect(sentBody(put)).toEqual({ expectedVersion: schedule.version, draftVersion: "opened", sendAt: future.toISOString(),
    changes: { subject: "Tuesday interview" } });
});

it("cancels only a scheduled send, using its displayed version", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [schedule, { ...schedule, id: "oms_confirming", status: "confirming" }] }));
  const cancel = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...schedule, status: "cancelled" }));
  renderWithClient(<OutlookScheduledList onOpen={vi.fn()}/>);
  const buttons = await screen.findAllByRole("button", { name: "Cancel send" });
  expect(buttons).toHaveLength(1);
  if (buttons[0]) fireEvent.click(buttons[0]);
  await waitFor(() => { expect(cancel).toHaveBeenCalled(); });
  expect(sentBody(cancel)).toEqual({ version: 2 });
});

it("keeps the editor open when Escape is pressed during submission", async () => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
  const draft: Schemas["OutlookDraft"] = {
    messageId: "omg_test", version: "opened", subject: "Interview", body: "Tuesday is available.",
    to: ["nadia@example.test"], cc: [], bcc: [], hasAttachments: false, bodyTruncated: false,
    schedule: null, scheduleNeedsReview: false, webUrl: null,
  };
  vi.spyOn(api, "GET").mockResolvedValue(ok(draft));
  let finish = () => {};
  const put = vi.spyOn(api, "PUT").mockImplementation(() => new Promise((resolve) => {
    finish = () => { resolve(ok(schedule)); };
  }));
  const onClose = vi.fn();
  renderWithClient(<OutlookDraftEditor id="omg_test" onClose={onClose} />);
  fireEvent.click(await screen.findByRole("button", { name: "Send now" }));
  await waitFor(() => { expect(put).toHaveBeenCalledTimes(1); });
  fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
  expect(onClose).not.toHaveBeenCalled();
  finish();
  await waitFor(() => { expect(onClose).toHaveBeenCalledExactlyOnceWith("send"); });
});

it("opens later schedule pages and checks uncertain sends without submitting mail", async () => {
  const uncertain = { ...schedule, id: "oms_uncertain", status: "uncertain" as const };
  const get = vi.spyOn(api, "GET").mockResolvedValueOnce(ok({ items: [schedule], nextOffset: 50 }))
    .mockResolvedValue(ok({ items: [uncertain], nextOffset: null }));
  const resolve = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...uncertain, status: "needs_review" }));
  renderWithClient(<OutlookScheduledList onOpen={vi.fn()} />);
  fireEvent.click(await screen.findByRole("button", { name: "Next" }));
  fireEvent.click(await screen.findByRole("button", { name: "Check Outlook again" }));
  await waitFor(() => { expect(resolve).toHaveBeenCalledTimes(1); });
  expect(get).toHaveBeenCalledWith("/v1/outlook/scheduled", { params: { query: { offset: 50 } } });
  expect(resolve.mock.calls[0]?.[0]).toBe("/v1/outlook/scheduled/{sendId}/resolve");
  expect(sentBody(resolve)).toEqual({ version: 2 });
  fireEvent.click(screen.getByRole("button", { name: "Previous" }));
  await waitFor(() => { expect(get).toHaveBeenLastCalledWith("/v1/outlook/scheduled", { params: { query: { offset: 0 } } }); });
});
