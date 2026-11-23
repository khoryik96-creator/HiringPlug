import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { ApolloIntegration } from "./ApolloIntegration";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
    Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value(this: HTMLDialogElement) { this.open = false; } });
  }
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function serve(connection: object) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/me" ? { permissions: ["contacts.apollo:use"] } : connection,
  ))) as never);
}

describe("Apollo in Settings › Integrations", () => {
  it("says how to turn it on when the server is not set up for it", async () => {
    serve({ state: "disabled" });
    renderWithClient(<ApolloIntegration />);
    expect(await screen.findByText("Not set up on this server")).toBeTruthy();
    expect(screen.getByText(/HP_APOLLO_BROKER_URL/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Connect Apollo" })).toBeNull();
  });

  it("connects your own Apollo account", async () => {
    serve({ state: "disconnected" });
    renderWithClient(<ApolloIntegration />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect Apollo" }));
    expect(screen.getByRole("heading", { name: "Apollo connection" })).toBeTruthy(); // the connect dialog
  });

  it("shows who it is connected as", async () => {
    serve({ state: "connected", accountName: "siti@hyppies.example" });
    renderWithClient(<ApolloIntegration />);
    expect(await screen.findByText("Connected")).toBeTruthy();
    expect(screen.getByText("siti@hyppies.example")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Manage connection" })).toBeTruthy();
  });
});
