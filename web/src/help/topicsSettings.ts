// Help topics on Settings: your own account, the team, roles and access, and
// the agency's setup. Summarised from docs/features.md and access-control.md.
import type { HelpTopic } from "./help";

export const SETTINGS_TOPICS: HelpTopic[] = [
  {
    id: "your-account",
    title: "Your account",
    summary: "Your password, two-step sign-in, signature, notifications and favourites.",
    words: "account profile password two step 2fa authenticator sign in login signature favourites my dropdowns preferences",
    links: [{ label: "Your account", href: "#/account" }, { label: "My dropdowns", href: "#/my-dropdowns" }],
    sections: [
      {
        heading: "Your account",
        points: [
          "Click your name in the top bar (on a phone: Menu, then your name).",
          "Change your password there, and set up two-step sign-in: a code from an authenticator app after your password, with ten recovery codes.",
          "Forgot your password? \"Forgot your password?\" on the sign-in page emails you a link.",
        ],
      },
      {
        heading: "Your own choices",
        points: [
          "Star your favourite skills, industries and countries (☆ in a dropdown, or Settings › My dropdowns): they come first for you only.",
          "Choose the candidate list's columns with Columns, and drag the Dashboard's blocks. Only you see your choices.",
        ],
      },
    ],
  },
  {
    id: "team-roles",
    title: "Settings, team and roles",
    summary: "Invite your team, choose what each role may see and do, and set up the agency.",
    words: "settings team invite user users staff role roles access permission permissions admin owner recruiter sourcer viewer salary visible mandatory fields dropdowns bin backup privacy pdpa",
    links: [
      { label: "Team", href: "#/team" },
      { label: "Roles and access", href: "#/access" },
      { label: "General", href: "#/settings" },
      { label: "Data protection", href: "#/privacy" },
    ],
    sections: [
      {
        heading: "Roles",
        points: [
          "Everyone has a role: owner, admin, recruiter, sourcer, hiring manager or viewer.",
          "Owners and admins run the agency's settings; recruiters work candidates, jobs and clients and see salaries; sourcers find and add candidates without salaries; viewers only look.",
          "Salaries, fees and offers are hidden from roles without salary access, by the database itself.",
        ],
      },
      {
        heading: "Invite your team",
        needs: ["identity.team:manage"],
        points: [
          "Settings › Team › invite: their name, email and role. They get an email to choose a password.",
          "Change someone's role, put them in a team, or deactivate them when they leave (their records stay).",
        ],
      },
      {
        heading: "Roles and access",
        needs: ["identity.roles:manage"],
        points: [
          "Settings › Roles and access: what each role may do, and which candidates it sees (all, its team's or its own).",
          "You can require two-step sign-in for everyone.",
        ],
      },
      {
        heading: "Set up the agency",
        needs: ["settings:manage"],
        points: [
          "General: your agency's name (used in CV file names) and which fields must be filled in.",
          "Dropdowns & categories: sources, notice periods, company categories, contact roles and more. Industries has the industry list.",
          "Salary: who sees salary in emails, and how a package becomes the monthly salary.",
          "System shows background work (emails, CV reading) and what failed, with Try again.",
        ],
      },
      {
        heading: "Deleted records",
        needs: ["bin:manage"],
        points: ["A deleted candidate, company or contact goes to Settings › Bin for 30 days, where it can be restored; then it is deleted for good."],
      },
      {
        heading: "Data protection (PDPA)",
        needs: ["privacy:manage"],
        points: [
          "Each candidate's page has Data protection: consent given or withdrawn, and requests about their data. Withdrawn consent means do not contact.",
          "Settings › Data protection shows what needs deciding. HyPlug never deletes anything by itself.",
        ],
      },
      {
        heading: "Backups",
        needs: ["settings.backups:manage"],
        points: ["Settings › Backups: the nightly backup, \"make one now\" and downloads."],
      },
    ],
  },
];
