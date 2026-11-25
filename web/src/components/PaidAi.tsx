// Settings > System: the server's paid-AI switch. Paused, no AI call is made
// on the server, through any company's key or the server's own; CVs are
// still read by the rules, and search by meaning, PDF copies and scans go
// on. Only an owner of the agency that runs the server may turn it.
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { formatWhen } from "../labels";
import { aiStatusKey } from "./AiNotice";
import { ErrorMessage } from "./ErrorMessage";

type Status = Schemas["SystemStatus"];

export function PaidAi({ paidAi, statusKey }: { paidAi: Status["paidAi"]; statusKey: readonly unknown[] }) {
  const queryClient = useQueryClient();
  const set = useMutation({
    mutationFn: async (paused: boolean) => unwrap(await api.PUT("/v1/settings/system/paid-ai", { body: { paused } })),
    onSuccess: async (st) => {
      queryClient.setQueryData(statusKey, st);
      await queryClient.invalidateQueries({ queryKey: aiStatusKey });
    },
  });
  const { paused, pausedByServer, canChange } = paidAi;
  return (
    <section className="panel paid-ai" aria-label="Paid AI">
      <h3>Paid AI</h3>
      {paused ? (
        <p className="warning" role="status">
          <strong>Paused on this server</strong>{paidAi.changedAt && !pausedByServer ? ` since ${formatWhen(paidAi.changedAt)}` : ""}
          {paidAi.reason ? `: ${paidAi.reason}` : "."} No AI is asked, through any company&apos;s key or the server&apos;s own: CVs are
          read by the rules alone, and fit scores, CV formatting and the assistant wait. Reading text and scans, PDF copies and search by
          meaning go on.
        </p>
      ) : (
        <p className="muted">On: companies use the AI they chose in Settings &gt; AI provider, within their monthly allowance.</p>
      )}
      {pausedByServer && <p className="muted">HP_AI_PAID=off on this server keeps it paused, whatever is chosen here.</p>}
      <ErrorMessage error={set.error} />
      {canChange && !pausedByServer && (
        paused
          ? <button type="button" className="doc-btn" disabled={set.isPending} onClick={() => { set.mutate(false); }}>Turn paid AI back on</button>
          : <button type="button" className="doc-btn danger" disabled={set.isPending}
            onClick={() => { if (window.confirm("Pause paid AI for every company on this server? Nothing is lost; turn it back on here.")) set.mutate(true); }}>
            Pause paid AI
          </button>
      )}
      {!canChange && (
        <p className="muted small">
          Only an owner of the agency that runs this server can pause or restart it here: the agency the server&apos;s HP_BACKUP_TENANT
          setting names (or the only one on the server). On the server itself, <code>hp paid-ai on</code> turns it back on.
        </p>
      )}
    </section>
  );
}
