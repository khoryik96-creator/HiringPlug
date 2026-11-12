// A contact's role (Settings › Contact roles: hiring manager, HR…): its
// label and colour, the same on their card, in lists and in their details.
import type { useFieldSettings } from "./useFieldSettings";

type FieldSettings = ReturnType<typeof useFieldSettings>;

/** The grey of a contact whose role is not set. */
export const NO_ROLE_COLOR = "#6b7280";

export function roleColor(fs: FieldSettings, role: string | null): string {
  return role ? fs.color("contact_role", role) : NO_ROLE_COLOR;
}

export function roleLabel(fs: FieldSettings, role: string | null, unset = "Not set"): string {
  return role ? fs.label("contact_role", role) : unset;
}
