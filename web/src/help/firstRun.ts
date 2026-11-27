// The getting-started checklist for a new company: each step, who can do it,
// where it is done, and how to tell from the company's records that it is
// done (so it ticks itself; nothing is ticked by hand).
import { api, type Permission, unwrap } from "../api/client";

export type StepId = "company" | "team" | "email" | "job" | "candidates" | "ai";

export interface Step {
  id: StepId;
  title: string;
  text: string;
  href: string;
  /** The button's words. */
  action: string;
  /** All of these are needed to do it (and to check it). */
  needs: readonly Permission[];
  /** Whether it is done, from the records. */
  done: () => Promise<boolean>;
}

export const STEPS: readonly Step[] = [
  {
    id: "company", title: "Add your company's details", action: "Open Invoicing", href: "#/invoicing",
    text: "Your address, registration and bank details, for the invoices HyPlug sends. Your agency's name is under Settings › General.",
    needs: ["settings:manage", "placements:read"],
    done: async () => unwrap(await api.GET("/v1/invoice-settings")).version > 0,
  },
  {
    id: "team", title: "Invite your team", action: "Open Team", href: "#/team",
    text: "Add your recruiters and admins by email, each with their role.",
    needs: ["identity.team:manage"],
    done: async () => unwrap(await api.GET("/v1/team")).items.length > 1,
  },
  {
    id: "email", title: "Connect your email", action: "Connect email", href: "#/email-connection",
    text: "Outlook, Gmail or your mail server, so emails to candidates and clients go from your own address.",
    needs: ["emails:send"],
    done: async () => unwrap(await api.GET("/v1/mailbox")).connected,
  },
  {
    id: "job", title: "Add your first job", action: "Open Jobs", href: "#/jobs",
    text: "Drop the client's job description and the job is filled in from it.",
    needs: ["jobs:create", "jobs:read"],
    done: async () => unwrap(await api.GET("/v1/jobs", { params: { query: { limit: 1 } } })).items.length > 0,
  },
  {
    id: "candidates", title: "Import candidates", action: "Open Import", href: "#/import",
    text: "Drop CVs or LinkedIn PDFs, or bring in a spreadsheet (CSV or Excel).",
    needs: ["intake:create", "candidates:read"],
    done: async () => unwrap(await api.GET("/v1/candidates", { params: { query: { limit: 1 } } })).items.length > 0,
  },
  {
    id: "ai", title: "Set up the AI provider", action: "Open AI provider", href: "#/ai-provider",
    text: "Switch on CV reading help, match scores, AI search and formatted CVs with your provider's API key.",
    needs: ["aiproviders:manage"],
    done: async () => {
      const ai = unwrap(await api.GET("/v1/ai-providers"));
      return ai.active !== "off" && (ai.active !== "server" || ai.serverAvailable);
    },
  },
];

/** The steps this person can do, in order. */
export function stepsFor(permissions: readonly Permission[]): Step[] {
  return STEPS.filter((s) => s.needs.every((p) => permissions.includes(p)));
}
