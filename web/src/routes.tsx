// Every page of the signed-in app in one table: its address, the permission
// it needs, where it appears (the top bar or the Settings menu) and what it
// shows. The router, the top bar and the Settings menu all read this table,
// so a page cannot be reachable but missing from its menu, or listed for
// someone who may not open it.
//
// Pages load on first use (each its own file), so signing in does not
// download every page.
import { type ComponentType, lazy, type LazyExoticComponent, type ReactNode } from "react";

import type { Me, Permission } from "./api/client";

// eslint-disable-next-line @typescript-eslint/no-explicit-any -- React's own bound for lazy components
type AnyComponent = ComponentType<any>;
/** A page loaded on first use, keeping its props' types. */
function page<M extends Record<K, AnyComponent>, K extends keyof M>(load: () => Promise<M>, name: K): LazyExoticComponent<M[K]> {
  return lazy(async () => ({ default: (await load())[name] }));
}

const EmailConnection = page(() => import("./pages/EmailConnection"), "EmailConnection");
const Access = page(() => import("./pages/Access"), "Access");
const AiProvider = page(() => import("./pages/AiProvider"), "AiProvider");
const BookingHours = page(() => import("./components/BookingHours"), "BookingHours");
const CandidateDetail = page(() => import("./pages/CandidateDetail"), "CandidateDetail");
const CandidateList = page(() => import("./pages/CandidateList"), "CandidateList");
const ChangePassword = page(() => import("./pages/Team"), "ChangePassword");
const CompanyDetail = page(() => import("./pages/CompanyDetail"), "CompanyDetail");
const CompanyList = page(() => import("./pages/CompanyList"), "CompanyList");
const AiSearch = page(() => import("./pages/AiSearch"), "AiSearch");
const CompareCvs = page(() => import("./pages/CompareCvs"), "CompareCvs");
const ContactDetail = page(() => import("./pages/ContactDetail"), "ContactDetail");
const ContactList = page(() => import("./pages/ContactList"), "ContactList");
const CvTemplates = page(() => import("./pages/CvTemplates"), "CvTemplates");
const Automations = page(() => import("./pages/Automations"), "Automations");
const Dashboard = page(() => import("./pages/Dashboard"), "Dashboard");
const EmailTemplates = page(() => import("./pages/EmailTemplates"), "EmailTemplates");
const Invoicing = page(() => import("./pages/Invoicing"), "Invoicing");
const ImportPage = page(() => import("./pages/ImportPage"), "ImportPage");
const Integrations = page(() => import("./pages/Integrations"), "Integrations");
const JobDetail = page(() => import("./pages/JobDetail"), "JobDetail");
const JobList = page(() => import("./pages/JobList"), "JobList");
const Leads = page(() => import("./pages/Leads"), "Leads");
const Placements = page(() => import("./pages/Placements"), "Placements");
const Privacy = page(() => import("./pages/Privacy"), "Privacy");
const Reports = page(() => import("./pages/Reports"), "Reports");
const ScreeningCall = page(() => import("./pages/ScreeningCall"), "ScreeningCall");
const ReferenceCheck = page(() => import("./pages/ReferenceCheck"), "ReferenceCheck");
const Industries = page(() => import("./pages/Industries"), "Industries");
const Settings = page(() => import("./pages/Settings"), "Settings");
const System = page(() => import("./pages/System"), "System");
const BackupsPage = page(() => import("./pages/Backups"), "BackupsPage");
const Team = page(() => import("./pages/Team"), "Team");

/** What a page is given: who is signed in, and the record its address names. */
export type PageContext = { permissions: Permission[]; me: Me; id: string };

export type Route = {
  /** The section it belongs to (highlighted in its menu), e.g. "candidates". */
  section: string;
  /** The address it answers; a group captures a record's ID. */
  path: RegExp;
  /** Needed to open it; none: everyone signed in. */
  permission?: Permission;
  /** Left out for those who have this permission (they get the full page instead). */
  unless?: Permission;
  /** Listed in the top bar or the Settings menu, with this label. */
  menu?: { in: "main" | "settings"; label: string; href: string };
  render: (c: PageContext) => ReactNode;
};

const can = (c: PageContext, p: Permission) => c.permissions.includes(p);

export const ROUTES: Route[] = [
  // The top bar, in order.
  { section: "dashboard", path: /^(#\/?)?$/, permission: "pipeline:read", menu: { in: "main", label: "Dashboard", href: "#/" },
    render: (c) => <Dashboard permissions={c.permissions} /> },
  { section: "jobs", path: /^#\/jobs\/(jo_[0-9a-z]+)$/, render: (c) => <JobDetail key={c.id} id={c.id} permissions={c.permissions} /> },
  { section: "jobs", path: /^#\/jobs/, menu: { in: "main", label: "Jobs", href: "#/jobs" },
    render: (c) => <JobList canCreate={can(c, "jobs:create")} /> },
  { section: "candidates", path: /^#\/candidates\/ai-search$/, permission: "matching:create",
    render: (c) => <AiSearch canSeeSalary={can(c, "candidates.compensation:read")} canReadJd={can(c, "jobs:create")} /> },
  { section: "candidates", path: /^#\/candidates\/(cand_[0-9a-z]+)\/screening$/,
    render: (c) => <ScreeningCall key={c.id} candidateId={c.id} permissions={c.permissions} /> },
  { section: "candidates", path: /^#\/candidates\/(cand_[0-9a-z]+)\/reference-check$/,
    render: (c) => <ReferenceCheck key={c.id} candidateId={c.id} permissions={c.permissions} /> },
  { section: "candidates", path: /^#\/candidates\/(cand_[0-9a-z]+)\/compare$/, render: (c) => <CompareCvs key={c.id} candidateId={c.id} /> },
  { section: "candidates", path: /^#\/candidates\/(cand_[0-9a-z]+)$/,
    render: (c) => <CandidateDetail key={c.id} id={c.id} permissions={c.permissions} /> },
  { section: "candidates", path: /^#\/candidates/, menu: { in: "main", label: "Candidates", href: "#/candidates" },
    render: (c) => <CandidateList canCreate={can(c, "candidates:create")} canSeeSalary={can(c, "candidates.compensation:read")} canAiSearch={can(c, "matching:create")} canEmail={can(c, "emails:send")} /> },
  { section: "companies", path: /^#\/companies\/(comp_[0-9a-z]+)$/,
    render: (c) => <CompanyDetail key={c.id} id={c.id} permissions={c.permissions} /> },
  { section: "companies", path: /^#\/companies/, menu: { in: "main", label: "Companies", href: "#/companies" },
    render: (c) => <CompanyList canCreate={can(c, "companies:create")} /> },
  { section: "contacts", path: /^#\/contacts\/(cont_[0-9a-z]+)$/,
    render: (c) => <ContactDetail key={c.id} id={c.id} permissions={c.permissions} /> },
  { section: "contacts", path: /^#\/contacts/, menu: { in: "main", label: "Contacts", href: "#/contacts" },
    render: (c) => <ContactList canCreate={can(c, "contacts:create")} canEmail={can(c, "emails:send")} /> },
  { section: "placements", path: /^#\/placements/, permission: "placements:read", menu: { in: "main", label: "Placements", href: "#/placements" },
    render: (c) => <Placements permissions={c.permissions} /> },
  { section: "leads", path: /^#\/leads/, permission: "leads:read", menu: { in: "main", label: "Leads", href: "#/leads" },
    render: (c) => <Leads permissions={c.permissions} /> },
  { section: "reports", path: /^#\/reports/, permission: "reports:read", menu: { in: "main", label: "Reports", href: "#/reports" },
    render: () => <Reports /> },
  { section: "import", path: /^#\/import/, menu: { in: "main", label: "Import", href: "#/import" },
    render: (c) => <ImportPage permissions={c.permissions} /> },

  // Settings, in its menu's order.
  { section: "settings", path: /^#\/settings/, permission: "settings:manage", menu: { in: "settings", label: "General", href: "#/settings" },
    render: () => <Settings /> },
  { section: "industries", path: /^#\/industries/, permission: "settings:manage", menu: { in: "settings", label: "Industries", href: "#/industries" }, render: () => <Industries /> },
  { section: "ai-provider", path: /^#\/ai-provider/, permission: "aiproviders:manage", menu: { in: "settings", label: "AI provider", href: "#/ai-provider" },
    render: () => <AiProvider /> },
  // Everyone else sees where the AI is set up, and who may do it.
  { section: "ai-provider", path: /^#\/ai-provider/, unless: "aiproviders:manage", menu: { in: "settings", label: "AI provider", href: "#/ai-provider" },
    render: (c) => (
      <main>
        <h2>AI provider</h2>
        <section className="panel">
          <p>Only owners and admins choose the AI provider and enter its API key (DeepSeek, Claude or ChatGPT).</p>
          <p className="muted">You are signed in as {c.me.user.name}, {c.me.role.replace("_", " ")} at {c.me.tenant.name}. Ask an owner or admin, or sign in with their account.</p>
        </section>
      </main>
    ) },
  { section: "team", path: /^#\/team/, permission: "identity.team:manage", menu: { in: "settings", label: "Team", href: "#/team" },
    render: (c) => <Team myRole={c.me.role} myId={c.me.user.id} /> },
  { section: "access", path: /^#\/access/, permission: "identity.roles:manage", menu: { in: "settings", label: "Roles and access", href: "#/access" },
    render: () => <Access /> },
  { section: "privacy", path: /^#\/privacy/, permission: "privacy:manage", menu: { in: "settings", label: "Data protection", href: "#/privacy" },
    render: () => <Privacy /> },
  { section: "integrations", path: /^#\/integrations/, permission: "integrations:manage", menu: { in: "settings", label: "Integrations", href: "#/integrations" },
    render: () => <Integrations /> },
  { section: "cv-templates", path: /^#\/cv-templates/, permission: "cvformats:create", menu: { in: "settings", label: "CV templates", href: "#/cv-templates" },
    render: (c) => <CvTemplates canManage={can(c, "cvformats.templates:manage")} /> },
  { section: "email-connection", path: /^#\/email-connection/, permission: "emails:send", menu: { in: "settings", label: "Email connection", href: "#/email-connection" }, render: (c) => <EmailConnection canManageApps={can(c, "emails.apps:manage")} /> },
  { section: "email-templates", path: /^#\/email-templates/, permission: "emails:send", menu: { in: "settings", label: "Email templates", href: "#/email-templates" },
    render: (c) => <EmailTemplates canManage={can(c, "emails.templates:manage")} canCreate={can(c, "emails.templates:create")} canPreview={can(c, "emails:send") && can(c, "candidates:read")} /> },
  { section: "automations", path: /^#\/automations/, permission: "automations:read", menu: { in: "settings", label: "Automations", href: "#/automations" },
    render: (c) => <Automations permissions={c.permissions} /> },
  { section: "invoicing", path: /^#\/invoicing/, permission: "placements:read", menu: { in: "settings", label: "Invoicing", href: "#/invoicing" },
    render: (c) => <Invoicing canManage={can(c, "settings:manage")} /> },
  { section: "system", path: /^#\/system/, permission: "settings:manage", menu: { in: "settings", label: "System", href: "#/system" },
    render: () => <System /> },
  { section: "backups", path: /^#\/backups/, permission: "settings.backups:manage", menu: { in: "settings", label: "Backups", href: "#/backups" },
    render: () => <BackupsPage /> },
  { section: "account", path: /^#\/account/, menu: { in: "settings", label: "Your account", href: "#/account" },
    render: (c) => <main><h2>Your account</h2>{can(c, "scheduling:use") && <BookingHours />}<ChangePassword /></main> },
];

const allowed = (r: Route, permissions: readonly Permission[]) =>
  (!r.permission || permissions.includes(r.permission)) && !(r.unless && permissions.includes(r.unless));

/** The entries of a menu this user may open, in order. */
export function menu(which: "main" | "settings", permissions: readonly Permission[]): Route[] {
  return ROUTES.filter((r) => r.menu?.in === which && allowed(r, permissions));
}

/**
 * The page an address opens for this user, and the record ID it names. An
 * address they may not open shows their start page instead (Settings: its
 * first part they may open; elsewhere the dashboard, or the candidates).
 */
export function resolve(hash: string, permissions: readonly Permission[]): { route: Route; id: string } {
  const path = hash.split("?")[0] ?? hash; // "#/candidates/cand_1?hl=SAP": the page, then what it is given
  for (const r of ROUTES) {
    const m = r.path.exec(path);
    if (m && allowed(r, permissions)) return { route: r, id: m[1] ?? "" };
  }
  if (/^#\/settings/.test(path)) {
    const parts = menu("settings", permissions);
    const first = parts.find((r) => !r.unless) ?? parts[0]; // a part they can use, not one that says who can
    if (first) return { route: first, id: "" };
  }
  const start = ROUTES.find((r) => r.section === "dashboard" && allowed(r, permissions))
    ?? ROUTES.find((r) => r.path.source === "^#\\/candidates");
  if (!start) throw new Error("no start page");
  return { route: start, id: "" };
}
