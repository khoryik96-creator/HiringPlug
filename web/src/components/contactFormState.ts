// The contact edit form's state, its checks, and its conversion to API requests.
import type { Contact, Schemas } from "../api/client";
import type { PickedCompany } from "./CompanyPicker";
import { orNull } from "./forms";

export interface ContactFormState {
  company: PickedCompany | null;
  firstName: string;
  lastName: string;
  title: string;
  email: string;
  phone: string;
  mobile: string;
  linkedinUrl: string;
  status: Schemas["ContactStatus"];
  role: string; // a key of the contact_role list; "" when not set
  notes: string;
}

/** Where the API reports a problem with each input (the VALIDATION_FAILED field path). */
export const CONTACT_PATHS: Record<keyof ContactFormState, string> = {
  company: "companyId", firstName: "firstName", lastName: "lastName", title: "title", email: "email",
  phone: "phone", mobile: "mobile", linkedinUrl: "linkedinUrl", status: "status", role: "role", notes: "notes",
};

export function formFromContact(c: Contact | null, company: PickedCompany | null = null): ContactFormState {
  return {
    company: c ? { id: c.companyId, name: c.companyName } : company,
    firstName: c?.firstName ?? "",
    lastName: c?.lastName ?? "",
    title: c?.title ?? "",
    email: c?.email ?? "",
    phone: c?.phone ?? "",
    mobile: c?.mobile ?? "",
    linkedinUrl: c?.linkedinUrl ?? "",
    status: c?.status ?? "active",
    role: c?.role ?? "",
    notes: c?.notes ?? "",
  };
}

/** Mistakes worth catching before sending; the API checks everything again. */
export function contactProblems(f: ContactFormState): Record<string, string> {
  const problems: Record<string, string> = {};
  if (!f.company) problems[CONTACT_PATHS.company] = "Choose the company they work at.";
  if (f.firstName.trim() === "") problems[CONTACT_PATHS.firstName] = "Enter a first name.";
  return problems;
}

/** The fields of a create or update request: every field is sent; empty inputs clear it. */
export function contactFields(f: ContactFormState) {
  return {
    companyId: f.company?.id ?? "",
    firstName: f.firstName.trim(),
    lastName: f.lastName.trim(),
    title: orNull(f.title),
    email: orNull(f.email),
    phone: orNull(f.phone),
    mobile: orNull(f.mobile),
    linkedinUrl: orNull(f.linkedinUrl),
    status: f.status,
    role: orNull(f.role),
    notes: orNull(f.notes),
  };
}
