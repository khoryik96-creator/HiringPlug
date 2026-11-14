import { useQuery } from "@tanstack/react-query";
import { useDeferredValue } from "react";

import { api, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";

/**
 * Before sending: recipients who are contacts that have left, or whose
 * mailbox does not exist (the email would bounce). A warning, not a block.
 */
export function RecipientWarnings({ text }: { text: string }) {
  const deferred = useDeferredValue(text);
  const emails = [...new Set(deferred.split(/[\s,;]+/).filter((a) => /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(a)).map((a) => a.toLowerCase()))].sort().slice(0, 100);
  const check = useQuery({
    queryKey: [...contactsKey, "address-check", emails],
    queryFn: async () => unwrap(await api.POST("/v1/contacts/address-check", { body: { emails } })).items,
    enabled: emails.length > 0,
    staleTime: 30_000,
    retry: false, // without contacts:read there is simply no warning
  });
  if (!check.data?.length) return null;
  return (
    <div className="warning recipient-warnings full" role="status">
      {check.data.map((p) => (
        <p key={p.contactId}>
          <a href={`#/contacts/${p.contactId}`}>{p.contactName}</a> ({p.email}){" "}
          {p.problem === "left" ? `has left ${p.companyName}: this email will not reach them.` : "will bounce: their mail server says the mailbox does not exist."}
        </p>
      ))}
    </div>
  );
}
