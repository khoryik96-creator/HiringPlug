import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Contact, unwrap } from "../api/client";
import { contactKey, contactsKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

/**
 * For a contact without an email: the address their colleagues' addresses
 * suggest (first.last@ ...), to use; it is then verified with the mail server.
 */
export function EmailSuggestion({ contact, canEdit }: { contact: Contact; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const suggestion = useQuery({
    queryKey: [...contactKey(contact.id), "email-suggestion"],
    queryFn: async () => unwrap(await api.GET("/v1/contacts/{contactId}/email-suggestion", { params: { path: { contactId: contact.id } } })).suggestion,
    enabled: !contact.email,
  });
  const use = useMutation({
    mutationFn: async (email: string) =>
      unwrap(await api.PATCH("/v1/contacts/{contactId}", { params: { path: { contactId: contact.id } }, body: { version: contact.version, email } })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(contactKey(contact.id), saved);
      await queryClient.invalidateQueries({ queryKey: contactsKey });
    },
  });
  const s = suggestion.data;
  if (contact.email || !s) return null;
  return (
    <div className="email-suggestion" role="status">
      Suggested email: <strong>{s.email}</strong>{" "}
      <span className="muted">({s.matches} of {s.of} colleagues&apos; addresses are spelled {s.pattern}@)</span>
      {canEdit && (
        <>
          {" "}
          <button type="button" className="small-button" disabled={use.isPending} onClick={() => { use.mutate(s.email); }}>
            {use.isPending ? "Saving…" : "Use and verify"}
          </button>
        </>
      )}
      <ErrorMessage error={use.error} />
    </div>
  );
}
