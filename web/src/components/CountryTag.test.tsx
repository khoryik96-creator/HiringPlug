import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { CountryTag, withCountry } from "./CountryTag";

afterEach(cleanup);

it("puts a company's country after its name", () => {
  expect(withCountry("Contoh Berhad", "MY")).toBe("Contoh Berhad [MY]");
  expect(withCountry("Contoh Pte Ltd", "sg")).toBe("Contoh Pte Ltd [SG]");
  expect(withCountry("Contoh", null)).toBe("Contoh");
  expect(withCountry("Contoh", "Malaysia"), "only a two-letter code").toBe("Contoh");
  render(<CountryTag country="MY" />);
  expect(screen.getByText("[MY]").getAttribute("title")).toBe("Malaysia");
});
