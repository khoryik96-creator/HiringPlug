import { describe, expect, it } from "vitest";

import { guessCompanyName } from "./companyNameGuess";

describe("a new company's name, guessed from its addresses", () => {
  it("comes from its LinkedIn page, words apart", () => {
    expect(guessCompanyName("", "https://www.linkedin.com/company/maju-foods-berhad/about/")).toBe("Maju Foods Berhad");
    expect(guessCompanyName("majufoods.com.my", "linkedin.com/company/maju-foods")).toBe("Maju Foods");
    expect(guessCompanyName("", "linkedin.com/company/123456"), "a number names no one").toBe("");
  });

  it("or else from its website", () => {
    expect(guessCompanyName("https://www.maybank2u.com.my/home", "")).toBe("Maybank2u");
    expect(guessCompanyName("grab.com", "")).toBe("Grab");
    expect(guessCompanyName("careers.petronas.com", "")).toBe("Petronas");
    expect(guessCompanyName("www.bnm.gov.my", "")).toBe("Bnm");
    expect(guessCompanyName("localhost", "")).toBe("");
    expect(guessCompanyName("", "")).toBe("");
  });
});
