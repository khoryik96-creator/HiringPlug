import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { STANDARD } from "../components/stages/columns";
import { StageTemplates } from "./StageTemplates";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const tech: Schemas["StageTemplate"] = {
  id: "stpl_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 2, name: "Tech hiring", isDefault: true, jobCount: 3, createdAt: "", updatedAt: "",
  steps: [{ key: "applied", label: "Applied", stage: "applied", category: "applied" }, { key: "tech", label: "Tech test", stage: "interview", category: "interview" }, { key: "placed", label: "Placed", stage: "placed", category: "placed" }],
};
const CATEGORIES: Schemas["StageCategories"] = {
  version: "0",
  items: [
    { key: "applied", label: "Applied", kind: "applied" }, { key: "screening", label: "Screening", kind: "screening" },
    { key: "submitted", label: "Submitted", kind: "submitted" }, { key: "interview", label: "1st Interview", kind: "interview" },
    { key: "interview_2", label: "2nd Interview", kind: "interview" }, { key: "interview_final", label: "Final Interview", kind: "interview" },
    { key: "offer", label: "Offer", kind: "offer" }, { key: "placed", label: "Placed", kind: "placed" },
  ],
};
const serve = (items: Schemas["StageTemplate"][], categories: Schemas["StageCategories"] | null = CATEGORIES) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/stage-categories"
    ? (categories ? ok(categories) : ok({ items: [], version: "0" }))
    : path === "/v1/me" ? ok({ permissions: [] }) : ok({ items, standard: STANDARD }))) as never);

it("lists the templates, the default first, with the standard stages", async () => {
  serve([tech]);
  renderWithClient(<StageTemplates />);
  const card = await screen.findByRole("region", { name: "Tech hiring" });
  expect(within(card).getByText("Default")).toBeTruthy();
  expect(within(within(card).getByRole("list", { name: "Stages" })).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Applied", "Tech test", "Placed"]);
  expect(within(card).getByText(/New jobs follow it\. 3 jobs use it\./)).toBeTruthy();
  expect(screen.getByText("Standard stages")).toBeTruthy();
});

it("saves a new template from the standard stages, with a stage added", async () => {
  serve([]);
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok(tech));
  renderWithClient(<StageTemplates />);
  fireEvent.click(await screen.findByRole("button", { name: "New template" }));
  const form = screen.getByRole("form", { name: "New template" });
  fireEvent.change(within(form).getByRole("textbox", { name: "Name" }), { target: { value: "Tech hiring" } });
  fireEvent.click(within(form).getByRole("checkbox", { name: "Default for new jobs" }));
  fireEvent.click(within(form).getByRole("button", { name: "+ Add a stage" }));
  fireEvent.change(within(form).getByRole("textbox", { name: "Stage 6 name" }), { target: { value: "Reference check" } });
  fireEvent.change(await within(form).findByRole("combobox", { name: "Stage 6 falls under" }), { target: { value: "offer" } });
  // A stage falls under a category: the third one, the 2nd interview round.
  fireEvent.change(within(form).getByRole("combobox", { name: "Stage 4 falls under" }), { target: { value: "interview_2" } });
  fireEvent.click(within(form).getByRole("button", { name: "Save template" }));
  await waitFor(() => { expect(post).toHaveBeenCalled(); });
  const body = sentBody(post) as Schemas["StageTemplateInput"];
  expect(body.name).toBe("Tech hiring");
  expect(body.isDefault).toBe(true);
  expect(body.steps.map((s) => `${s.label}/${s.stage}/${s.category ?? ""}`)).toEqual([
    "Applied/applied/applied", "Screening/screening/screening", "Submitted/submitted/submitted", "Interview/interview/interview_2",
    "Offer/offer/offer", "Reference check/offer/offer", "Placed/placed/placed",
  ]); // added before Placed
});

it("edits a template with the version it showed, and shows the API's field errors", async () => {
  serve([tech]);
  const put = vi.spyOn(api, "PUT").mockResolvedValue(invalidField("steps.1.label", "another stage has this name"));
  renderWithClient(<StageTemplates />);
  fireEvent.click(within(await screen.findByRole("region", { name: "Tech hiring" })).getByRole("button", { name: "Edit" }));
  const form = screen.getByRole("form", { name: "Edit Tech hiring" });
  fireEvent.click(within(form).getByRole("button", { name: "Move Tech test up" }));
  fireEvent.click(within(form).getByRole("button", { name: "Save template" }));
  expect(await within(form).findByText("another stage has this name")).toBeTruthy();
  const body = sentBody(put) as Schemas["StageTemplateUpdate"];
  expect(body.version).toBe(2);
  expect(body.steps.map((s) => s.key)).toEqual(["tech", "applied", "placed"]);
  expect(within(form).getByRole("textbox", { name: "Stage 2 name" }).getAttribute("aria-invalid")).toBe("true");
});

it("deletes a template after asking, saying the jobs using it keep their stages", async () => {
  serve([tech]);
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  const del = vi.spyOn(api, "DELETE").mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) });
  renderWithClient(<StageTemplates />);
  fireEvent.click(within(await screen.findByRole("region", { name: "Tech hiring" })).getByRole("button", { name: "Delete" }));
  expect(confirm.mock.calls[0]?.[0]).toBe('Delete the template "Tech hiring"? The 3 jobs using it keep their stages.');
  await waitFor(() => { expect(del).toHaveBeenCalled(); });
});
