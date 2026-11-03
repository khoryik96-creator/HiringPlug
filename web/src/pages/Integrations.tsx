import { ApiKeys } from "../components/ApiKeys";
import { Webhooks } from "../components/Webhooks";

/** Connecting other systems: API keys and webhooks (owners and admins). */
export function Integrations() {
  return (
    <>
      <h2>Integrations</h2>
      <ApiKeys />
      <Webhooks />
    </>
  );
}
