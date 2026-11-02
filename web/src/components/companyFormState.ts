// The company edit form's state, its checks, and its conversion to API requests.
import type { Company, Schemas } from "../api/client";
import { orNull } from "./forms";

export interface CompanyFormState {
  name: string;
  type: Schemas["CompanyType"];
  website: string;
  industry: string;
  size: "" | Schemas["CompanySize"];
  phone: string;
  address: string;
  city: string;
  country: string;
  registrationNumber: string;
  about: string;
}

/** Where the API reports a problem with each input (the VALIDATION_FAILED field path). */
export const COMPANY_PATHS: Record<keyof CompanyFormState, string> = {
  name: "name", type: "type", website: "website", industry: "industry", size: "size", phone: "phone",
  address: "address", city: "city", country: "country", registrationNumber: "registrationNumber", about: "about",
};

export function formFromCompany(c: Company | null): CompanyFormState {
  return {
    name: c?.name ?? "",
    type: c?.type ?? "client",
    website: c?.website ?? "",
    industry: c?.industry ?? "",
    size: c?.size ?? "",
    phone: c?.phone ?? "",
    address: c?.address ?? "",
    city: c?.city ?? "",
    country: c?.country ?? "",
    registrationNumber: c?.registrationNumber ?? "",
    about: c?.about ?? "",
  };
}

/** Mistakes worth catching before sending; the API checks everything again. */
export function companyProblems(f: CompanyFormState): Record<string, string> {
  const problems: Record<string, string> = {};
  if (f.name.trim() === "") problems[COMPANY_PATHS.name] = "Enter the company's name.";
  const country = f.country.trim();
  if (country !== "" && !/^[A-Za-z]{2}$/.test(country)) {
    problems[COMPANY_PATHS.country] = "Use the 2-letter country code, e.g. MY for Malaysia.";
  }
  return problems;
}

/** The fields of a create or update request: every field is sent; empty inputs clear it. */
export function companyFields(f: CompanyFormState): Omit<Schemas["CompanyUpdate"], "version"> & { name: string } {
  return {
    name: f.name.trim(),
    type: f.type,
    website: orNull(f.website),
    industry: orNull(f.industry),
    size: f.size === "" ? null : f.size,
    phone: orNull(f.phone),
    address: orNull(f.address),
    city: orNull(f.city),
    country: orNull(f.country.toUpperCase()),
    registrationNumber: orNull(f.registrationNumber),
    about: orNull(f.about),
  };
}
