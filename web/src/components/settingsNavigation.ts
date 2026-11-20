import type { Permission } from "../api/client";
import { resolve, type Route } from "../routes";

export const settingsGroups = [
  { label: "My settings", sections: ["account", "my-dropdowns"] },
  { label: "Recruiting setup", sections: ["settings-stages", "settings-lists", "industries", "settings-salary", "cv-templates"] },
  { label: "Email & scheduling", sections: ["email-connection", "email-templates", "booking-calendars", "automations"] },
  { label: "Team & agency", sections: ["settings", "team", "access", "privacy"] },
  { label: "Finance", sections: ["invoicing", "settings-claims-storage"] },
  { label: "System & integrations", sections: ["integrations", "ai-provider", "companion", "system", "backups", "bin"] },
];

const aliases: Record<string, string> = {
  account: "profile personal preferences change password signature",
  "my-dropdowns": "my options presets personal choices",
  "settings-stages": "pipeline recruitment workflow stage templates applied screening interview offer",
  "settings-lists": "dropdown options candidate source job types company categories contact roles benefits expense claim types",
  industries: "industry subindustry subindustries taxonomy edit rename",
  "settings-salary": "compensation pay fixed monthly package breakdown screening bonus allowance epf",
  "cv-templates": "format resume résumé CV Studio document styling",
  "email-connection": "connect outlook microsoft gmail google SMTP mailbox inbox sign in login OAuth Entra application registration",
  "email-templates": "candidate email messages banner canned replies",
  "booking-calendars": "team interview availability scheduling bookings calendar",
  automations: "rules automatic workflow reminders email triggers",
  settings: "agency name branding required fields",
  team: "invite users staff teammates recruiter onboarding",
  access: "permissions user access admin roles visibility",
  privacy: "privacy consent retention data deletion do not contact",
  invoicing: "invoice billing placement fees finance",
  "settings-claims-storage": "receipt files scans expenses claim uploads OneDrive Google Drive Dropbox",
  integrations: "connections API service webhooks third party",
  "ai-provider": "AI assistant model API key OpenAI Claude DeepSeek",
  companion: "browser extension LinkedIn CV profile import",
  system: "configuration uptime application status background jobs retries",
  backups: "database restore export data backup recovery",
  bin: "recycle trash deleted records restore",
};

const subsections: Record<string, { label: string; id: string; words: string }[]> = {
  settings: [
    { label: "Mandatory fields", id: "mandatory-fields", words: "require required candidate job fields information" },
  ],
  "settings-salary": [
    { label: "Salary in emails", id: "salary-in-emails", words: "see salary compensation visibility sent email roles" },
    { label: "Salary calculation", id: "salary-calculation", words: "salary formula fixed bonus allowances monthly equivalent screening breakdown" },
  ],
  "settings-lists": [
    { label: "Notice period", id: "notice-period", words: "dropdown options notice days months immediately" },
    { label: "Company categories", id: "company-categories", words: "signed client prospect VIP colour" },
    { label: "Contact roles", id: "contact-roles", words: "hiring manager HR contact colour categories" },
    { label: "Company benefits template", id: "benefits-template", words: "benefit client perks pre offer process steps" },
    { label: "Expense claims", id: "claims", words: "receipt and claim types dropdown options" },
  ],
};

const features = [
  { section: "outlook", label: "Outlook Mail", href: "#/outlook", words: "inbox messages drafts forwarding schedule mail reply" },
  { section: "calendar", label: "Calendar", href: "#/calendar", words: "day week month agenda events outlook calendar dock" },
  { section: "import", label: "Import profiles", href: "#/import", words: "upload CV CVs resume resumes bulk parse documents create candidates", permission: "intake:create" },
] satisfies { section: string; label: string; href: string; words: string; permission?: Permission }[];

export function groupSettings(pages: Route[]) {
  const ungrouped = new Set(pages);
  const groups = settingsGroups.map(({ label, sections }) => {
    const links = sections.flatMap((section) => pages.filter((route) => route.section === section));
    for (const link of links) ungrouped.delete(link);
    return { label, pages: links };
  });
  return [...groups, { label: "Other settings", pages: [...ungrouped] }].filter((group) => group.pages.length);
}

type SearchEntry = { label: string; href: string; group: string; words: string };

export function settingsSearchEntries(groups: ReturnType<typeof groupSettings>, permissions: readonly Permission[]): SearchEntry[] {
  const settings = groups.flatMap(({ label: group, pages }) => pages.flatMap(({ section, menu }) => {
    if (!menu) return [];
    return [
      { label: menu.label, href: menu.href, group, words: aliases[section] ?? "" },
      ...(subsections[section] ?? []).map(({ label, id, words }) => ({ label, href: `${menu.href}#${id}`, group: menu.label, words })),
    ];
  }));
  return [
    ...settings,
    ...features.filter((feature) =>
      (!feature.permission || permissions.includes(feature.permission)) &&
      resolve(feature.href, permissions).route.section === feature.section
    ).map((feature) => ({ ...feature, group: "Features" })),
  ];
}

const normalize = (value: string) => value.normalize("NFD").replace(/\p{Diacritic}/gu, "").toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();
const stopWords = new Set("a an i me my can could how do where what who which want to the for in and with is edit change add update find".split(" "));

export function searchSettings(entries: SearchEntry[], query: string) {
  const phrase = normalize(query);
  if (!phrase) return [];
  const tokens = [...new Set(phrase.split(" ").filter((token) => !stopWords.has(token)))];
  if (!tokens.length) return [];
  return entries.flatMap((entry) => {
    const label = normalize(entry.label);
    const labelWords = label.split(" ");
    const keywords = normalize(`${entry.words} ${entry.group}`).split(" ");
    let score = label === phrase ? 100 : label.startsWith(phrase) ? 30 : 0;
    for (const token of tokens) {
      if (labelWords.includes(token)) score += 5;
      else if (labelWords.some((word) => word.startsWith(token))) score += 3;
      else if (keywords.includes(token)) score += 2;
      else if (keywords.some((word) => word.startsWith(token))) score += 1;
      else return [];
    }
    return [{ entry, score }];
  }).sort((a, b) => b.score - a.score).map(({ entry }) => entry);
}
