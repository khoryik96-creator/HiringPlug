// A candidate's job title and employer can come from a CV as one long word
// ("DatabaseAdministrator · HongLeongBank(PetalingJaya,Malaysia)"): the
// header tile wraps it instead of running out over the next tile.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { afterEach, expect, it } from "vitest";

afterEach(() => { document.head.innerHTML = ""; document.body.innerHTML = ""; });

it("wraps a header value with no spaces inside its tile", () => {
  const style = document.createElement("style");
  style.textContent = ["theme.css", "styles.css"].map((f) => readFileSync(join(process.cwd(), "src", f), "utf8")).join("\n");
  document.head.append(style);
  document.body.innerHTML = `<div class="candidate-banner"><div class="banner-strip"><div class="fact">
    <span class="label">Job title</span><span class="figure">DatabaseAdministrator · HongLeongBank(PetalingJaya,Malaysia)</span>
  </div></div></div>`;
  const tile = document.querySelector<HTMLElement>(".fact");
  const value = document.querySelector<HTMLElement>(".figure");
  if (!tile || !value) throw new Error("not rendered");
  expect(getComputedStyle(value).overflowWrap).toBe("anywhere");
  expect(getComputedStyle(tile).minWidth).toBe("0px");
});
