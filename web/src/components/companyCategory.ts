// A company's category (Settings › Company categories): its label and colour,
// and the colours drawn from it for chips and the company's card.

/** The category of a company without one. */
export const UNKNOWN_CATEGORY = "unknown";

/** A card's background: the colour, pale, shading to near white (companies, contacts and candidates alike). */
export function cardGradient(hex: string, from = tint(hex, 0.72)): string {
  return `linear-gradient(120deg, ${from}, ${tint(hex, 0.9)})`;
}

/** A colour mixed with white: 0 is the colour, 1 is white. */
export function tint(hex: string, toWhite: number): string {
  const n = parseInt(hex.replace("#", ""), 16);
  if (!/^#[0-9a-fA-F]{6}$/.test(hex) || Number.isNaN(n)) return "#f3f4f6";
  const mix = (c: number) => Math.round(c + (255 - c) * toWhite);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map(mix);
  return `#${[r, g, b].map((c) => (c ?? 0).toString(16).padStart(2, "0")).join("")}`;
}

/** A chip in the category's colour: a pale fill, the colour's own border and a dark enough text. */
export function chipStyle(color: string): { background: string; borderColor: string; color: string } {
  return { background: tint(color, 0.85), borderColor: color, color: darken(color) };
}

/** The colour, darker, for text on its pale tint. */
export function darken(hex: string): string {
  const n = parseInt(hex.replace("#", ""), 16);
  if (!/^#[0-9a-fA-F]{6}$/.test(hex) || Number.isNaN(n)) return "#1B2230";
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => Math.round(c * 0.55));
  return `#${[r, g, b].map((c) => (c ?? 0).toString(16).padStart(2, "0")).join("")}`;
}
