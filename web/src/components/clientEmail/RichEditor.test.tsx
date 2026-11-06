import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RichEditor } from "./RichEditor";

afterEach(() => { cleanup(); });

const fields = [{ token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true }];

describe("the message box", () => {
  it("inserts a field where the cursor was, even after a toolbar dropdown took the focus", () => {
    const onChange = vi.fn();
    render(<RichEditor html="<p>She expects  per month.</p>" resetKey={0} onChange={onChange} fields={fields} banners={[]} bannerUrls={{}} />);
    const box = screen.getByRole("textbox", { name: "Message" });
    const text = box.querySelector("p")?.firstChild as Text;
    const range = document.createRange();
    range.setStart(text, "She expects ".length);
    range.collapse(true);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
    fireEvent.mouseUp(box); // the cursor is remembered

    const dropdown = screen.getByLabelText("Insert field");
    window.getSelection()?.removeAllRanges(); // the dropdown took the focus
    dropdown.focus();
    fireEvent.change(dropdown, { target: { value: "[Expected salary]" } });
    expect(box.textContent).toBe("She expects [Expected salary] per month.");
    expect(onChange).toHaveBeenLastCalledWith(box.innerHTML);
  });

  it("adds at the end when the cursor was never in it", () => {
    render(<RichEditor html="<p>Hi</p>" resetKey={0} onChange={vi.fn()} fields={fields} banners={[]} bannerUrls={{}} />);
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "[Expected salary]" } });
    expect(screen.getByRole("textbox", { name: "Message" }).textContent).toBe("Hi[Expected salary]");
  });
});
