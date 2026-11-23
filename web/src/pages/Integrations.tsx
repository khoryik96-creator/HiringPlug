import { ApiKeys } from "../components/ApiKeys";
import { ApolloIntegration } from "../components/ApolloIntegration";
import { Webhooks } from "../components/Webhooks";

/** Connecting other systems: Apollo (work emails), API keys and webhooks (owners and admins). */
export function Integrations() {
  return (
    <main>
      <h2>Integrations</h2>
      <ApolloIntegration />
      <ApiKeys />
      <Webhooks />
    </main>
  );
}
