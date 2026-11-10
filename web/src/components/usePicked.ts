// People ticked in a list, to email each separately: kept by id, in the order ticked.
import { useState } from "react";

import type { ContactSummary } from "../api/client";
import type { Person } from "./SeparateEmail";

/** A contact as someone to email. */
export const contactPerson = (c: ContactSummary) => ({ id: c.id, person: { contactId: c.id, name: c.fullName, email: c.email } });

export function usePicked() {
  const [picked, setPicked] = useState<Map<string, Person>>(new Map());
  const pick = (people: { id: string; person: Person }[], on: boolean) => {
    setPicked((cur) => {
      const next = new Map(cur);
      for (const { id, person } of people) {
        if (on) next.set(id, person);
        else next.delete(id);
      }
      return next;
    });
  };
  return { picked, ids: new Set(picked.keys()), people: [...picked.values()], pick, clear: () => { setPicked(new Map()); } };
}
