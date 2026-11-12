import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { useState } from "react";
import { CandidateTabs, type CandidateTab } from "./CandidateTabs";

afterEach(cleanup);
function Workspace() {
  const [active, setActive] = useState<CandidateTab>("details");
  return <><button onClick={() => { setActive("email"); }}>Write email</button><CandidateTabs active={active} onChange={setActive} sections={[
    {id: "details", label: "Details", content: <input aria-label="Profile draft" defaultValue="Original"/>},
    {id: "cv", label: "CV & files", content: <p>Viewer mounted</p>},
    {id: "email", label: "Email", content: <textarea aria-label="Email draft"/>},
  ]}/></>;
}
it("loads tabs on demand and keeps drafts including a tab opened by the page action", () => {
  render(<Workspace/>);
  expect(screen.queryByText("Viewer mounted")).toBeNull();
  expect(screen.getByRole("tab", {name: "Details"}).getAttribute("aria-selected")).toBe("true");
  fireEvent.change(screen.getByLabelText("Profile draft"), {target: {value: "Unsaved profile"}});
  fireEvent.click(screen.getByRole("button", {name: "Write email"}));
  fireEvent.change(screen.getByLabelText("Email draft"), {target: {value: "Unsaved email"}});
  fireEvent.click(screen.getByRole("tab", {name: "Details"}));
  expect(screen.getByLabelText<HTMLInputElement>("Profile draft").value).toBe("Unsaved profile");
  fireEvent.click(screen.getByRole("tab", {name: "Email"}));
  expect(screen.getByLabelText<HTMLTextAreaElement>("Email draft").value).toBe("Unsaved email");
});
it("supports keyboard navigation and focuses the selected tab", () => {
  render(<Workspace/>);
  fireEvent.keyDown(screen.getByRole("tab", {name: "Details"}), {key: "ArrowRight"});
  expect(document.activeElement).toBe(screen.getByRole("tab", {name: "CV & files"}));
  expect(screen.getByText("Viewer mounted")).toBeTruthy();
  fireEvent.keyDown(document.activeElement as HTMLElement, {key: "End"});
  expect(document.activeElement).toBe(screen.getByRole("tab", {name: "Email"}));
  fireEvent.keyDown(document.activeElement as HTMLElement, {key: "Home"});
  expect(document.activeElement).toBe(screen.getByRole("tab", {name: "Details"}));
});
