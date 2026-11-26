import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { ok, renderWithClient } from "../../test-utils";
import { OutlookAddressInput, typedAddress, withAddress } from "./OutlookAddressInput";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function Box() {
  const [value, setValue] = useState("");
  return <><label htmlFor="to">To</label><OutlookAddressInput id="to" value={value} onChange={setValue} /></>;
}

it("knows what is being typed, and puts the chosen address in its place", () => {
  expect(typedAddress("a@x.test, nad")).toBe("nad");
  expect(typedAddress("nad")).toBe("nad");
  expect(withAddress("a@x.test, nad", "nadia@example.test")).toBe("a@x.test, nadia@example.test, ");
  expect(withAddress("a@x.test;nad", "nadia@example.test")).toBe("a@x.test; nadia@example.test, ");
  expect(withAddress("nad", "nadia@example.test")).toBe("nadia@example.test, ");
});

it("suggests the people in their mail as they type, by address or name, and fills one in", async () => {
  const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
    { address: "nadia@example.test", name: "Nadia Rahman" }, { address: "nadine@client.test", name: null },
  ] }));
  renderWithClient(<Box />);
  const to = screen.getByLabelText<HTMLInputElement>("To");
  fireEvent.change(to, { target: { value: "nad" } });
  expect(await screen.findByRole("option", { name: /Nadia Rahman/ })).toBeTruthy();
  expect(get).toHaveBeenCalledWith("/v1/outlook/addresses", expect.objectContaining({ params: { query: { q: "nad" } } }));
  fireEvent.keyDown(to, { key: "ArrowDown" });
  expect(screen.getByRole("option", { name: "nadine@client.test" }).getAttribute("aria-selected")).toBe("true");
  fireEvent.keyDown(to, { key: "Enter" });
  expect(to.value).toBe("nadine@client.test, ");
  // The next one, by clicking it; one already in is not offered again.
  fireEvent.change(to, { target: { value: "nadine@client.test, na" } });
  fireEvent.mouseDown(await screen.findByRole("option", { name: /Nadia Rahman/ }));
  expect(to.value).toBe("nadine@client.test, nadia@example.test, ");
  fireEvent.change(to, { target: { value: "nadine@client.test, nadia@example.test, n" } });
  await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/outlook/addresses", expect.objectContaining({ params: { query: { q: "n" } } })); });
  expect(screen.queryByRole("option")).toBeNull();
});

it("asks nothing until something is typed, and Escape closes only the list", async () => {
  const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ address: "nadia@example.test", name: null }] }));
  renderWithClient(<Box />);
  const to = screen.getByLabelText("To");
  fireEvent.focus(to);
  expect(get).not.toHaveBeenCalled();
  fireEvent.change(to, { target: { value: "n" } });
  await screen.findByRole("option");
  const escape = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
  to.dispatchEvent(escape);
  await waitFor(() => { expect(screen.queryByRole("option")).toBeNull(); });
  expect(escape.defaultPrevented).toBe(true); // a window it is in stays open
});
