import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { Owner } from "./Owner";
import { renderWithClient } from "../test-utils";

afterEach(cleanup);
const save = () => Promise.resolve();

it("as a chip, shows the owner's initials, and no made-up initials for nobody", () => {
  renderWithClient(<Owner chip what="job" ownerId="usr_1" ownerName="Guok Wei" canChange={false} save={save} />);
  expect(screen.getByLabelText("Guok Wei").textContent).toBe("GW");
  cleanup();
  renderWithClient(<Owner chip what="job" ownerId={null} ownerName={null} canChange={false} save={save} />);
  expect(screen.getByText("Nobody")).toBeTruthy();
  expect(screen.queryByLabelText("Nobody")).toBeNull(); // not an "N" avatar
  expect(document.querySelector(".no-owner")).toBeTruthy();
});

it("keeps \"Owner:\" outside a chip", () => {
  renderWithClient(<Owner what="candidate" ownerId={null} ownerName={null} canChange={false} save={save} />);
  expect(screen.getByText(/Owner:/)).toBeTruthy();
});
