// A quiet button marked danger (doc-btn, pill-btn, secondary, link) shows red
// text on its own background; only a plain danger button is solid red. With
// both, the text was red on red and the button read as a blank red block.
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { afterEach, expect, it } from "vitest";

afterEach(() => { document.head.innerHTML = ""; document.body.innerHTML = ""; });

it("never paints a danger button's text in its own background colour", () => {
  const css = ["theme.css", "styles.css", "folders.css", "search-reader.css"].map((f) => readFileSync(join(process.cwd(), "src", f), "utf8")).join("\n");
  const style = document.createElement("style");
  style.textContent = css;
  document.head.append(style);
  for (const cls of ["danger", "doc-btn danger", "pill-btn danger", "secondary danger", "link danger"]) {
    const b = document.createElement("button");
    b.className = cls;
    b.textContent = "Delete";
    document.body.append(b);
    const look = getComputedStyle(b);
    expect(look.color, cls).not.toBe(look.backgroundColor);
    if (cls !== "danger") expect(look.backgroundColor, `${cls}: not the solid red`).not.toBe("rgb(185, 28, 28)");
  }
});
