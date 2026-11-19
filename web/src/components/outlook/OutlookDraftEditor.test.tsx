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
