// The values a message's fields take for whom it is about (the composer puts
// a field in as its value). Salary fields' values are asked for only once one
// is wanted, because each answer with a salary is counted in the log of who
// was shown a salary: opening the composer does not ask; a salary field in
// the message or the subject (inserted, typed, or from a template) does.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { editorText } from "./richText";

type Request = Omit<Schemas["EmailPreviewRequest"], "includeSalary">;

const preview = async (body: Request, includeSalary: boolean) =>
  unwrap(await api.POST("/v1/email-previews", { body: includeSalary ? { ...body, includeSalary } : body }));

/** Whether a message or subject holds one of these field tokens ("[Expected salary]"), as typed (any case). */
export function mentionsField(text: string, tokens: readonly string[]): boolean {
  const lower = text.toLowerCase();
  return tokens.some((t) => lower.includes(t.toLowerCase()));
}

/**
 * The field values for `body` (whom the email is about), under `key`; with the salary fields' too once
 * the message (`html`) or `subject` holds one that has a value. `withheld` are the salary fields with a
 * value not asked for yet: put in as their token, they are filled once their value comes.
 */
export function useFieldValues({ key, body, html, subject = "", enabled = true }: {
  key: readonly unknown[]; body: Request; html: string; subject?: string; enabled?: boolean;
}) {
  const base = useQuery({ queryKey: key, queryFn: () => preview(body, false), enabled });
  const withheld = base.data?.withheldFields ?? [];
  const wanted = withheld.length > 0 && mentionsField(`${subject}\n${editorText(html)}`, withheld);
  // Asked once a salary field is wanted; kept for this key after it is filled in (no token left).
  const salary = useQuery({
    queryKey: [...key, "salary"], queryFn: () => preview(body, true), enabled: enabled && wanted, refetchOnWindowFocus: false,
  });
  const data = salary.data ?? base.data;
  return {
    data,
    values: data?.fieldValues,
    withheld: salary.data ? [] : withheld,
    /** A salary field's value is being asked for (it goes in once it comes). */
    filling: wanted && salary.isFetching,
    error: base.error ?? salary.error,
    isPending: base.isPending,
  };
}

/** A candidate's field values with their salary's, asked for now (counted as shown): salary details to put in. */
export function salaryValues(body: Request): Promise<Schemas["EmailPreview"]> {
  return preview(body, true);
}
