import { cleanup, fireEvent, screen } from "@testing-library/react";
import type { SyntheticEvent } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { renderWithClient } from "../test-utils";
import { EmailRecipients } from "./EmailRecipients";

afterEach(cleanup);

it("reveals Cc and Bcc independently without submitting the email", () => {
  const change = vi.fn(), submit = vi.fn((event: SyntheticEvent) => { event.preventDefault(); });
  renderWithClient(<form onSubmit={submit}><EmailRecipients to="client@example.test" cc="" bcc="" errors={{}} onChange={change}/></form>);
  expect(screen.queryByRole("textbox", { name: "Cc" })).toBeNull();
  expect(screen.queryByRole("textbox", { name: "Bcc" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Cc" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Cc" }), { target: { value: "copy@example.test" } });
  expect(change).toHaveBeenLastCalledWith("cc", "copy@example.test");
  expect(screen.queryByRole("textbox", { name: "Bcc" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Bcc" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Bcc" }), { target: { value: "private@example.test" } });
  expect(change).toHaveBeenLastCalledWith("bcc", "private@example.test");
  expect(submit).not.toHaveBeenCalled();
});

it("always shows filled recipients and fields with validation errors", () => {
  renderWithClient(<EmailRecipients to="client@example.test" cc="copy@example.test" bcc="" errors={{ bcc: "Enter a valid email address" }} onChange={vi.fn()}/>);
  expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Cc" }).value).toBe("copy@example.test");
  expect(screen.getByRole("textbox", { name: "Bcc" }).getAttribute("aria-invalid")).toBe("true");
  expect(screen.getByText("Enter a valid email address")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cc" })).toBeNull();
});
