// A company's country after its name: "Contoh Berhad [MY]", with the
// country's name on hover. Several companies share a name across countries.
import { regionName } from "../labels";

/** "[MY]" for a two-letter code; "" without one. */
export function countryCode(country: string | null | undefined): string {
  const code = (country ?? "").trim().toUpperCase();
  return /^[A-Z]{2}$/.test(code) ? code : "";
}

/** A company's name with its country, as text (pickers, titles). */
export function withCountry(name: string, country: string | null | undefined): string {
  const code = countryCode(country);
  return code ? `${name} [${code}]` : name;
}

export function CountryTag({ country }: { country: string | null | undefined }) {
  const code = countryCode(country);
  if (!code) return null;
  return <span className="country-tag" title={regionName(code) ?? code}>[{code}]</span>;
}
