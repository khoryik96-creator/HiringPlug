import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { ApolloLookupDialog } from "./FindHRApollo";

type Lookup = Schemas["ApolloLookup"];
type LookupRequest = Schemas["ApolloLookupRequest"];
const person = { url: "https://www.linkedin.com/in/farah", name: "Farah Aziz", title: "Recruiter" } as Schemas["HrScanPerson"];
const confirmation = "a".repeat(64);

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
    Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value(this: HTMLDialogElement) { this.open = false; } });
  }
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function serve(answer: (body: LookupRequest, call: number) => Lookup) {
  let calls = 0;
  return vi.spyOn(api, "POST").mockImplementation(((_path: string, init: { body: LookupRequest }) =>
    Promise.resolve(ok(answer(init.body, ++calls)))) as never);
}

function open() {
  renderWithClient(<ApolloLookupDialog companyId="comp_1" companyName="Maju Bank" person={person} onClose={vi.fn()} openConnection={vi.fn()} />);
}

describe("Apollo work email", () => {
  it("offers a fresh confirmation when a status check finds the lookup never reached Apollo", async () => {
    const post = serve((body) => body.action === "check" ? { state: "cancelled", reason: "retryable_limit" }
      : body.action === "preview" && post.mock.calls.length > 2 ? { state: "cancelled", reason: "retryable_limit", confirmation }
        : { state: "unconfirmed" });
    open();
    fireEvent.click(await screen.findByRole("button", { name: "Check lookup status" }));
    expect(await screen.findByRole("button", { name: "Confirm credit use" })).toBeTruthy();
  });

  it("says which refusal Apollo gave when it limits a lookup", async () => {
    serve(() => ({ state: "limited", reason: "plan_not_allowed" }));
    open();
    expect(await screen.findByText(/its plan may not allow email lookups through other apps/)).toBeTruthy();
    expect(screen.queryByText(/Check your plan, credits, and account activity/)).toBeNull();
    cleanup();
    serve(() => ({ state: "limited" })); // an older broker gives no reason
    open();
    expect(await screen.findByText(/Check your plan, credits, and account activity/)).toBeTruthy();
  });

  it("keeps checking a lookup confirmed after the dialog has been open a while", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let state: Lookup["state"] = "ready";
    const post = serve((body) => {
      if (body.action === "unlock") state = "queued";
      return state === "ready" ? { state, confirmation } : { state };
    });
    open();
    const confirm = await screen.findByRole("button", { name: "Confirm credit use" });
    await act(async () => { await vi.advanceTimersByTimeAsync(80_000); }); // read, thought it over
    fireEvent.click(confirm);
    await screen.findByText(/Queued/);
    const before = post.mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(3_000); });
    await waitFor(() => { expect(post.mock.calls.length).toBeGreaterThan(before); });
    expect(screen.queryByRole("button", { name: "Refresh status" })).toBeNull();
  });
});
