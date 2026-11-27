import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { stageName } from "../components/stages/categories";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { StageCategories } from "./StageCategories";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const cats: Schemas["StageCategories"] = {
  version: "1764200000000000",
  items: [
    { key: "applied", label: "Applied", kind: "applied" }, { key: "screening", label: "Screening", kind: "screening" },
    { key: "submitted", label: "Submitted", kind: "submitted" }, { key: "interview", label: "1st Interview", kind: "interview" },
    { key: "interview_2", label: "2nd Interview", kind: "interview" }, { key: "interview_final", label: "Final Interview", kind: "interview" },
    { key: "offer", label: "Offer", kind: "offer" }, { key: "placed", label: "Placed", kind: "placed" },
  ],
};
const serve = (permissions: string[]) => vi.spyOn(api, "GET").mockImplementation(((path: string) =>
  Promise.resolve(ok(path === "/v1/me" ? { permissions } : cats))) as never);

it("shows the categories by kind; without the right there is nothing to change", async () => {
  serve(["pipeline:read"]);
  renderWithClient(<StageCategories />);
  const interviews = await screen.findByText("Counts as interview");
  expect(within(interviews.parentElement as HTMLElement).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["1st Interview", "2nd Interview", "Final Interview"]);
  expect(screen.queryByRole("button", { name: "Edit categories" })).toBeNull();
});

it("renames, adds and orders categories within their kind, and saves them with the version shown", async () => {
  serve(["pipeline:read", "pipeline.categories:manage"]);
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(cats));
  renderWithClient(<StageCategories />);
  fireEvent.click(await screen.findByRole("button", { name: "Edit categories" }));
  const form = screen.getByRole("form", { name: "Edit categories" });
  const interviews = within(form).getByRole("group", { name: "Counts as interview" });
  fireEvent.change(within(interviews).getByRole("textbox", { name: "Interview category 1 name" }), { target: { value: "First round" } });
  fireEvent.click(within(interviews).getByRole("button", { name: "+ Add an interview category" }));
  fireEvent.change(within(interviews).getByRole("textbox", { name: "Interview category 4 name" }), { target: { value: "Technical Test" } });
  fireEvent.click(within(interviews).getByRole("button", { name: "Move Technical Test up" }));
  expect(within(form).getByRole("button", { name: "Remove Applied" }).hasAttribute("disabled")).toBe(true); // each kind keeps one
  fireEvent.click(within(form).getByRole("button", { name: "Save categories" }));
  await waitFor(() => { expect(put).toHaveBeenCalled(); });
  const body = sentBody(put) as Schemas["StageCategoriesInput"];
  expect(body.version).toBe("1764200000000000");
  expect(body.items.filter((c) => c.kind === "interview")).toEqual([
    { key: "interview", label: "First round", kind: "interview" }, { key: "interview_2", label: "2nd Interview", kind: "interview" },
    { label: "Technical Test", kind: "interview" }, { key: "interview_final", label: "Final Interview", kind: "interview" },
  ]);
  expect(body.items).toHaveLength(9);
});

it("names a candidate's stage by its category", () => {
  expect(stageName("interview", "interview_2", cats.items)).toBe("2nd Interview");
  expect(stageName("interview", null, cats.items)).toBe("1st Interview"); // none recorded: the kind's first
  expect(stageName("rejected", null, cats.items)).toBe("Rejected");
  expect(stageName("interview", "interview_2", undefined)).toBe("Interview"); // loading
});
