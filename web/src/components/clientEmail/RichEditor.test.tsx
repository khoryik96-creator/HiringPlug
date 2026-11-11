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

  it("puts a field in as the candidate's value, editable, and fills a field typed in full", () => {
    const onChange = vi.fn();
    const values = { "[Expected salary]": "RM 12,000 per month" };
    render(<RichEditor html="<p>Hi</p>" resetKey={0} onChange={onChange} fields={fields} banners={[]} bannerUrls={{}} values={values} />);
    const box = screen.getByRole("textbox", { name: "Message" });
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "[Expected salary]" } });
    expect(box.innerHTML).toContain('<span data-field="candidate.expectedSalary">RM 12,000 per month</span>');
    // The cursor is after the value, not in it: what is typed next is not part of it.
    const sel = window.getSelection();
    expect(sel?.anchorNode?.parentElement?.closest("[data-field]")).toBeNull();

    // Edited in the message: kept as typed.
    const span = box.querySelector("span[data-field]");
    if (!span) throw new Error("no value");
    span.textContent = "RM 12,500 (negotiable)";
    fireEvent.input(box);
    expect(onChange).toHaveBeenLastCalledWith(expect.stringContaining(">RM 12,500 (negotiable)</span>"));

    // Typed in full: becomes the value, any case.
    box.insertAdjacentHTML("beforeend", "<p>Expects [expected SALARY] now</p>");
    fireEvent.input(box);
    expect(box.querySelectorAll("span[data-field]")).toHaveLength(2);
    expect(box.textContent).toContain("Expects RM 12,000 per month\u200b now");
  });

  it("fills a template's fields when it is put in", () => {
    const onChange = vi.fn();
    render(<RichEditor html="<p>[Expected salary] and [Unknown]</p>" resetKey={1} onChange={onChange} fields={fields} banners={[]} bannerUrls={{}}
      values={{ "[Expected salary]": "RM 9,000 per month" }} />);
    expect(screen.getByRole("textbox", { name: "Message" }).textContent).toBe("RM 9,000 per month\u200b and [Unknown]"); // the cursor mark goes when sent
    expect(onChange).toHaveBeenCalled();
  });

  it("puts something asked for (a booking link) where the cursor was", () => {
    const props = { resetKey: 0, onChange: vi.fn(), fields, banners: [], bannerUrls: {} };
    const { rerender } = render(<RichEditor html="<p>Hi Siti,</p><p>Regards, Aina</p>" {...props} />);
    const box = screen.getByRole("textbox", { name: "Message" });
    const range = document.createRange();
    range.setStartAfter(box.querySelector("p") as Node);
    range.collapse(true);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
    fireEvent.mouseUp(box);
    window.getSelection()?.removeAllRanges(); // the booking form took the focus
    rerender(<RichEditor html="" {...props} inserting={{ id: 1, html: "<p>Pick a time: https://ats.example/book/abc</p>" }} />);
    expect(box.textContent).toBe("Hi Siti,Pick a time: https://ats.example/book/abcRegards, Aina");
  });
});

it("outlines invalid messages and clears corrected candidate placeholders while retaining salary protection",()=>{
 render(<RichEditor html='<table><tbody><tr><td><span data-field="candidate.currentSalary" data-missing-detail="Current salary">—</span></td></tr></tbody></table>' resetKey={0} invalid onChange={vi.fn()} fields={fields} banners={[]} bannerUrls={{}}/>);
 const editor=screen.getByRole("textbox",{name:"Message"});expect(editor.getAttribute("aria-invalid")).toBe("true");
 const missing=editor.querySelector("[data-missing-detail]");if(!missing)throw new Error("Missing highlight");missing.textContent="RM 9,000";fireEvent.input(editor);
 expect(editor.querySelector("[data-missing-detail]")).toBeNull();expect(editor.querySelector('[data-field="candidate.currentSalary"]')?.textContent).toBe("RM 9,000");
});
