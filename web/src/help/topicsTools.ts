// Help topics on clients, communication and the tools around the work:
// companies and contacts, email, WhatsApp, folders, duplicates,
// notifications, exports, reports, AI, automations, claims and leads.
// Summarised from docs/features.md; keep them short and in the screens' words.
import type { HelpTopic } from "./help";

export const TOOL_TOPICS: HelpTopic[] = [
  {
    id: "companies-contacts",
    title: "Companies and contacts",
    summary: "Your clients and the people there: add them, fill them in from the website, verify emails.",
    words: "client clients company companies contact contacts hiring manager hr email verifier verify website autofill terms of business find hr",
    needs: ["companies:read", "contacts:read"],
    links: [{ label: "Companies", href: "#/companies" }, { label: "Contacts", href: "#/contacts" }],
    sections: [
      {
        heading: "Companies",
        needs: ["companies:read"],
        points: [
          "Companies › Add company. Typing its name suggests it; \"Fill in from website\" reads its website for the details.",
          "A company's page has its contacts, jobs, terms of business (fee rates by salary), benefits and documents.",
        ],
      },
      {
        heading: "Contacts",
        needs: ["contacts:read"],
        points: [
          "Contacts › Add contact, or add one from the company's page. Their LinkedIn link fills in their name and title.",
          "Verify email checks whether an address would receive mail; a contact who left the company is flagged.",
          "A contact's page has Emails: write to them with your templates and see what was sent.",
        ],
      },
    ],
  },
  {
    id: "emails",
    title: "Emails and templates",
    summary: "Connect your mailbox, write emails from any record, and use templates with fields.",
    words: "email mail outlook gmail microsoft google smtp connect mailbox send template templates fields signature bulk mass mailshot client ai write reply",
    needs: ["emails:send"],
    links: [
      { label: "Email connection", href: "#/email-connection" },
      { label: "Email templates", href: "#/email-templates" },
      { label: "Outlook Mail", href: "#/outlook" },
    ],
    sections: [
      {
        heading: "Connect your email",
        points: [
          "Settings › Email connection: sign in with Microsoft (Outlook) or Google, or enter your mail server's details. Emails then go from your own address.",
          "Your signature is under Settings › Your account.",
        ],
      },
      {
        heading: "Write an email",
        points: [
          "\"Email candidate\" on a candidate's page, \"Write email\" on a contact's page, or tick several on a list and choose Email each separately: each person gets their own copy.",
          "\"Email client\" sends a client one or more candidates with the details you choose and their CVs attached.",
          "Insert field puts a field such as [First name] where the cursor is; it is filled in for each person.",
          "With the AI set up, ✦ AI writes a draft, improves yours or answers their email.",
        ],
      },
      {
        heading: "Templates",
        points: [
          "Settings › Email templates: the agency's templates, with fields, ready in every email window.",
          "Outlook Mail (the envelope icon) shows your mailbox inside HyPlug.",
        ],
      },
    ],
  },
  {
    id: "whatsapp",
    title: "WhatsApp",
    summary: "Message a candidate or contact on WhatsApp, from a template.",
    words: "whatsapp chat message text phone mobile wa me template",
    needs: ["emails:send"],
    links: [{ label: "WhatsApp templates", href: "#/whatsapp-templates" }],
    sections: [
      {
        heading: "Send a WhatsApp message",
        points: [
          "The green chat button beside a candidate's phone or a contact's mobile opens a message: blank or from a template, filled in with their name and the job.",
          "\"Open in WhatsApp\" opens your own WhatsApp with the message typed in, and keeps it on their timeline as a WhatsApp note.",
          "HyPlug sends nothing itself, so no WhatsApp Business account is needed. Salary is never filled in.",
          "Settings › WhatsApp templates holds the templates (three to start with).",
        ],
      },
    ],
  },
  {
    id: "folders",
    title: "Folders",
    summary: "Group candidates, companies or contacts in folders and share them.",
    words: "folder folders group list shortlist share tabs collection",
    needs: ["folders:use"],
    links: [{ label: "Candidates", href: "#/candidates" }, { label: "Companies", href: "#/companies" }, { label: "Contacts", href: "#/contacts" }],
    sections: [
      {
        heading: "Use folders",
        points: [
          "Folders are the tabs above the Candidates, Companies and Contacts lists. \"+ New folder\" makes one.",
          "Choose rows (click, Shift+click, Ctrl+A), then Add to folder; or drag a row by its name onto a folder.",
          "Search and filters still work inside a folder. Deleting a folder never deletes the records in it.",
          "Share a folder with anyone in your agency; they can add and remove records, only you rename, share or delete it.",
        ],
      },
    ],
  },
  {
    id: "duplicates",
    title: "Duplicates and merging",
    summary: "Spot the same person or company twice and merge them into one.",
    words: "duplicate duplicates double twice same merge combine dedupe dedup similar",
    needs: ["candidates:read", "companies:read", "contacts:read"],
    links: [{ label: "Duplicate companies", href: "#/companies/duplicates" }, { label: "Duplicate contacts", href: "#/contacts/duplicates" }],
    sections: [
      {
        heading: "Candidates",
        needs: ["candidates:read"],
        points: [
          "Two candidates cannot have the same email: adding one names the candidate who has it.",
          "A candidate's page warns about others with the same phone or a very similar name.",
          "Merge moves the duplicate's CVs, jobs and notes into the one you keep and fills its empty fields.",
        ],
      },
      {
        heading: "Companies and contacts",
        needs: ["companies:read", "contacts:read"],
        points: [
          "A company's or contact's page warns about possible duplicates (the same website, LinkedIn page or mobile, or a name much alike).",
          "\"Possible duplicates\" beside the Companies and Contacts search lists every group across the agency.",
          "\"Merge into this one\" keeps one record and moves the other's contacts, jobs, emails, notes and documents to it.",
        ],
      },
    ],
  },
  {
    id: "notifications",
    title: "Notifications: the bell and the morning email",
    summary: "What needs you today, in the top bar and by email at 8:00.",
    words: "notification notifications bell alert alerts reminder morning email digest new applicants tasks due",
    needs: ["notifications:read"],
    links: [{ label: "Your account", href: "#/account" }],
    sections: [
      {
        heading: "The bell",
        points: [
          "The bell in the top bar lists what needs you: new applicants on your jobs, tasks due today or overdue, offers waiting for your approval and your interviews today and tomorrow.",
          "A red number shows what is new since you last opened it. Each item opens the candidate.",
        ],
      },
      {
        heading: "The morning email",
        points: [
          "At 8:00 you get the same list by email, only on mornings when there is something.",
          "Settings › Your account › Notifications has a switch for each kind and for the morning email.",
        ],
      },
    ],
  },
  {
    id: "exports",
    title: "Export to Excel or CSV",
    summary: "Download what a list or report shows as an Excel or CSV file.",
    words: "export exports download excel xlsx csv spreadsheet backup list report sheet",
    needs: ["candidates:export", "companies:export", "contacts:export", "jobs:export", "placements:export", "reports:read"],
    links: [{ label: "Candidates", href: "#/candidates" }, { label: "Jobs", href: "#/jobs" }, { label: "Reports", href: "#/reports" }],
    sections: [
      {
        heading: "Export a list",
        points: [
          "Export (Excel .xlsx or CSV) is on Candidates, Companies, Contacts, Jobs, Placements and Reports.",
          "It gives what the page's search and filters find (for candidates, the advanced search too), up to 10,000 rows. Narrow the search for more.",
          "Salaries and fees are in the file only for roles that may see them, and every export is in the audit log.",
          "By default owners and admins export candidates, companies, contacts and placements; recruiters may export jobs. Settings › Roles and access can change this.",
        ],
      },
    ],
  },
  {
    id: "reports",
    title: "Dashboard and reports",
    summary: "This week's numbers, who waits too long, conversion, sources, placements and time to fill.",
    words: "report reports dashboard numbers kpi conversion sources time to fill placements per recruiter stats statistics market map",
    needs: ["pipeline:read", "reports:read"],
    links: [{ label: "Dashboard", href: "#/" }, { label: "Reports", href: "#/reports" }],
    sections: [
      {
        heading: "The Dashboard",
        needs: ["pipeline:read"],
        points: [
          "The week's numbers, what needs you today, your tasks, upcoming interviews and your open jobs with how many are at each stage.",
          "Drag a block by its handle to put it where you want it; \"Reset order\" puts them back.",
          "Click a number in the open jobs to open that job at that stage.",
        ],
      },
      {
        heading: "Reports",
        needs: ["reports:read"],
        points: [
          "More › Reports, for the last 90 days, this year, last year or your own dates.",
          "Pipeline conversion, applications and placements by source, placements and fees per recruiter, and time to fill.",
        ],
      },
    ],
  },
  {
    id: "ai",
    title: "AI features and the AI provider",
    summary: "The assistant, CV vs job match, AI search and formatted CVs, and how to switch the AI on.",
    words: "ai artificial intelligence assistant chatgpt openai claude deepseek qwen provider api key match score fit",
    links: [{ label: "AI provider", href: "#/ai-provider" }],
    sections: [
      {
        heading: "What the AI does",
        points: [
          "The AI assistant (a tab on candidate and job pages) answers questions about the record.",
          "Each candidate on a job gets a fit score with reasons quoting the job and the CV.",
          "AI search ranks candidates against a JD, and \"Make formatted CV\" lays a CV out in your template.",
          "Names, contact details and salaries are never sent to the AI.",
        ],
      },
      {
        heading: "Switch it on",
        needs: ["aiproviders:manage"],
        points: [
          "Settings › AI provider: choose DeepSeek, Claude, ChatGPT or Qwen and paste its API key. The key is checked straight away.",
          "Each company has a monthly AI allowance.",
        ],
      },
      {
        heading: "Who switches it on",
        points: ["Only owners and admins choose the AI provider; ask one if the AI features are off."],
      },
    ],
  },
  {
    id: "automations",
    title: "Automations",
    summary: "Emails and tasks that happen by themselves: when something happens, then do this.",
    words: "automation automations rule rules workflow automatic reminder task follow up check in",
    needs: ["automations:read"],
    links: [{ label: "Automations", href: "#/automations" }],
    sections: [
      {
        heading: "When, if, then",
        points: [
          "Settings › Automations: WHEN something happens (a candidate applies, moves stage, an interview is booked, a placement starts), IF it is a given client, THEN email someone or give the recruiter a task.",
          "Seven ready-made ones start switched off: switch on the ones you want.",
          "An automation sends on its own or asks first: its emails wait under To approve.",
          "Tasks show on the Dashboard under Your tasks.",
        ],
      },
    ],
  },
  {
    id: "claims",
    title: "Expense claims",
    summary: "Claim expenses from receipt photos, and approve your team's claims.",
    words: "claim claims expense expenses receipt receipts grab mileage parking reimbursement approve",
    needs: ["claims:create", "claims:approve"],
    links: [{ label: "Claims", href: "#/claims" }],
    sections: [
      {
        heading: "Claims",
        points: [
          "More › Claims: drop receipt photos or PDFs; each becomes a draft claim with the date, total and merchant read from it.",
          "Check the suggested values (striped until you change them), add the purpose and claim type, then submit.",
          "Approvers approve or reject with a reason, and mark approved claims paid.",
        ],
      },
      {
        heading: "Where receipts are kept",
        needs: ["claims.storage:manage"],
        points: ["Settings › Claims storage files every submitted receipt in your Google Drive, OneDrive or Dropbox."],
      },
    ],
  },
  {
    id: "leads",
    title: "Leads (job ads)",
    summary: "Companies that are hiring, from job board feeds and careers pages.",
    words: "leads lead job ads hiring business development bd prospects jobstreet",
    needs: ["leads:read"],
    links: [{ label: "Leads", href: "#/leads" }],
    sections: [
      {
        heading: "Leads",
        points: [
          "More › Leads collects job ads from job board feeds and companies' careers pages: companies that are hiring now.",
          "Sources adds a job board feed or a careers page (checked every day); \"+ Save a lead\" keeps an ad you found elsewhere.",
          "Mark each lead new, contacted, became a client or ignored, and link it to a company.",
        ],
      },
    ],
  },
];
