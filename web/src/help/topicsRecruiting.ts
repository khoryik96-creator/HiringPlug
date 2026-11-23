// Help topics on the daily recruiting work: candidates, search, jobs and the
// pipeline, interviews, offers, placements and invoices. Summarised from
// docs/features.md; keep them short and in the words the screens use.
import type { HelpTopic } from "./help";

export const RECRUITING_TOPICS: HelpTopic[] = [
  {
    id: "getting-started",
    title: "Getting started",
    summary: "The first things to set up, and how the app is laid out.",
    words: "start begin new setup set up first time checklist onboarding tour overview",
    links: [{ label: "Dashboard", href: "#/" }, { label: "Settings", href: "#/settings" }],
    sections: [
      {
        heading: "The getting-started checklist",
        needs: ["pipeline:read"],
        points: [
          "A new company sees a short checklist on the Dashboard: your company details, inviting your team, connecting your email, your first job, importing candidates and the AI provider.",
          "Each step links to its screen and ticks itself once it is done. You only see the steps your role can do.",
          "Hide it with \"Hide checklist\". To see it again, use \"Show the getting-started checklist\" at the bottom of this help page.",
        ],
      },
      {
        heading: "Finding your way",
        points: [
          "The top bar has the main pages: Dashboard, Jobs, Candidates, Companies, Contacts and Placements; Leads, Reports, Import and Claims are under More.",
          "The icons on the right open Outlook Mail, the Calendar, your notifications (the bell), this help (?) and Settings.",
          "Settings has a search box: type what you want to do, such as \"connect Outlook\" or \"who can see salary\".",
          "On a phone, the pages, your account and Sign out are under the Menu button.",
          "Your version of HiringPlug is shown beside its name in the top bar.",
        ],
      },
    ],
  },
  {
    id: "add-candidates",
    title: "Add and import candidates",
    summary: "From CVs, LinkedIn PDFs, the LinkedIn companion, emails and spreadsheets (CSV or Excel).",
    words: "upload cv cvs resume resumes import bulk drop new candidate linkedin pdf csv excel xlsx spreadsheet companion extension careers applicants",
    needs: ["intake:create", "candidates:create"],
    links: [{ label: "Candidates", href: "#/candidates" }, { label: "Import", href: "#/import" }, { label: "LinkedIn companion", href: "#/companion" }],
    sections: [
      {
        heading: "From CVs",
        needs: ["intake:create"],
        points: [
          "Drop CV files anywhere on the Candidates page, on the Dashboard's \"Import profiles\", or on More › Import. Click \"Drop CVs anywhere to upload\" to pick files instead.",
          "PDF, Word (.docx and .doc), text, RTF, OpenDocument and photos or scans of a CV are read; scans are read with OCR.",
          "Each CV becomes a candidate owned by you, filled in from the CV: name, email, phone, location, nationality, current job, skills, work history and education.",
          "One candidate per email: when the CV's email belongs to someone already there, the CV is added to that candidate instead.",
          "Choose the source first (\"Preset the source\"), e.g. LinkedIn or JobStreet. The import shows each file's progress with a link to each candidate.",
        ],
      },
      {
        heading: "From LinkedIn",
        needs: ["intake:create"],
        points: [
          "On a LinkedIn profile choose More › Save to PDF, then import the PDF like a CV.",
          "Or install the LinkedIn companion (Settings › LinkedIn companion) to add the profile you are looking at in one click.",
        ],
      },
      {
        heading: "From a spreadsheet (CSV or Excel)",
        needs: ["candidates:create"],
        points: [
          "More › Import › From a spreadsheet takes a CSV file or an Excel file (.xlsx; its first sheet). An old .xls file: save it as .xlsx first.",
          "Columns are matched to fields by their names (Full Name, E-mail, Mobile Number, ...); check and change the matches before importing.",
          "Every row gets the same checks as the forms: duplicates show as \"already there\" with a link, and rows to fix can be downloaded, corrected and imported again.",
          "Companies and contacts can be imported the same way; contacts find their company by name.",
        ],
      },
      {
        heading: "Other ways in",
        points: [
          "CVs attached to emails can be imported from Outlook Mail.",
          "People who apply on your careers page or through a job board link are added to the job by themselves.",
        ],
      },
    ],
  },
  {
    id: "search",
    title: "Search for candidates",
    summary: "Quick search, boolean search, the advanced filters and AI search.",
    words: "find look up boolean and or not near filters advanced ai search jd shortlist full resume phone email name keyword",
    needs: ["candidates:read"],
    links: [{ label: "Candidates", href: "#/candidates" }, { label: "AI search", href: "#/candidates/ai-search" }],
    sections: [
      {
        heading: "Quick search",
        points: [
          "The search on the Dashboard finds candidates, jobs, companies and contacts as you type.",
          "Type a name, an email or a phone number in any format (012-345 6789 or +60123456789). Separate several with commas.",
          "\"name - company\" (a dash with spaces) keeps only people who work or worked at that company.",
        ],
      },
      {
        heading: "Boolean search",
        points: [
          "Type AND, OR, NOT (in capitals) in the Candidates search bar: it turns blue with a BOOLEAN badge and searches CVs and profiles.",
          "Use \"quotes\" for a phrase, manag* for any ending, and SAP NEAR/3 planning for words close together.",
          "Example: SAP AND (BW OR BPC) NOT internship.",
        ],
      },
      {
        heading: "Advanced search (Filters)",
        points: [
          "The funnel opens Filters: status, work authorization, nationality, where they live, source, industry, IT skills, certifications, tags, notice period, expected salary and when they were last updated. \"Mine only\" is there too.",
          "Each result says whether the words were found in the CV or the profile, and opening a candidate marks the words in their CV.",
          "\"Full resume\" reads the results without leaving the list: the CV on the right, Prev and Next (or the arrow keys).",
        ],
      },
      {
        heading: "AI search",
        needs: ["matching:create"],
        points: [
          "Candidates › AI search: drop the client's JD (or paste it) and the AI ranks your best-matching candidates from 0 to 100, with reasons and gaps.",
          "It also writes a boolean for the job; \"Use it\" puts it in the search.",
          "Names, contact details and salaries are never sent to the AI. It needs the AI provider to be set up.",
        ],
      },
    ],
  },
  {
    id: "candidate-page",
    title: "A candidate's page and CVs",
    summary: "Status, work rights, CVs, reading details from a CV, formatted and blind CVs.",
    words: "profile cv files format formatted blind cv studio compare read details review status work authorization salary notes",
    needs: ["candidates:read"],
    links: [{ label: "Candidates", href: "#/candidates" }, { label: "CV templates", href: "#/cv-templates" }],
    sections: [
      {
        heading: "The card",
        points: [
          "The banner is coloured by the candidate's status: looking out, passive, not looking or placed. Click a status to change it.",
          "The stripe on the right shows their work rights (Local, PR, Spousal / RP-T, Needs pass, Not known); click to change.",
          "Click an email or phone number to copy it. Notes and activity are right under the banner.",
        ],
      },
      {
        heading: "CVs and files",
        points: [
          "CV & files shows each CV as it looks. \"CV beside\" keeps the CV on the right while you read the details.",
          "Read details reads a CV; Review details shows what the CV says next to the profile, and \"Use ticked details\" saves it.",
          "Compare side by side shows two CVs and what they disagree on: the check before sending a formatted CV.",
        ],
      },
      {
        heading: "Formatted and blind CVs",
        needs: ["cvformats:create"],
        points: [
          "\"Make formatted CV\" lays the CV out in your agency's template, from their CV (with the AI) or from their profile (with a Word template).",
          "Blind leaves out the name and contact details and describes employers instead of naming them.",
          "A candidate keeps up to three formatted and four other CVs; adding one more asks which to replace.",
        ],
      },
    ],
  },
  {
    id: "jobs-pipeline",
    title: "Jobs and the pipeline",
    summary: "Add a job (from the client's JD), move candidates through the stages, and advertise it.",
    words: "job order vacancy requisition jd job description stage stages board steps move submit shortlist reject withdraw careers page job board ad advert",
    needs: ["jobs:read"],
    links: [{ label: "Jobs", href: "#/jobs" }, { label: "Pipeline stages", href: "#/settings/stages" }],
    sections: [
      {
        heading: "Add a job",
        needs: ["jobs:create"],
        points: [
          "Jobs › Add job (the + button). Drop the client's job description (PDF or Word) and the form is filled in from it.",
          "A job has its company and hiring contact, title, status (open, on hold, filled, closed), location and work type.",
          "The salary budget is seen only by roles that may see salaries.",
        ],
      },
      {
        heading: "The pipeline",
        points: [
          "Each candidate on a job moves through the stages: applied, screening, submitted, interview, offer, placed (or rejected or withdrawn, with a reason).",
          "Steps shows the stages as a funnel with the people waiting longest first, each with a button for the usual next step. Board shows a column per stage: drag a line to move it.",
          "Click several people to email or move them together. Closed ones are listed below and can be reopened.",
          "Add someone to a job from their page (Add to job), from search results (+ Add to job), or under \"Possible candidates\" on the job.",
        ],
      },
      {
        heading: "Pipeline stages",
        needs: ["pipeline.templates:manage"],
        points: ["Settings › Pipeline stages changes the names and steps of your pipeline."],
      },
      {
        heading: "Advertise it",
        needs: ["jobs:update"],
        points: [
          "Write the job ad on the job's page; every ad is also shown blind (without the client's name).",
          "Published ads appear on your careers page, and each has a link to post on job boards; applicants come straight into the job.",
        ],
      },
    ],
  },
  {
    id: "interviews-calendar",
    title: "Interviews and the calendar",
    summary: "Schedule interviews, give feedback, use the calendar and booking links.",
    words: "interview schedule book meeting calendar outlook teams zoom meet feedback reminder booking link calendly availability ics",
    needs: ["interviews:read", "calendars:use", "scheduling:use"],
    links: [{ label: "Calendar", href: "#/calendar" }, { label: "Your account", href: "#/account" }],
    sections: [
      {
        heading: "Schedule an interview",
        needs: ["interviews:schedule"],
        points: [
          "On the candidate's job choose Interview: the date and time, Teams, Google Meet, Zoom, in person or by phone, the interviewers and instructions.",
          "An interviewer already busy at that time shows as a clash; \"Schedule anyway\" overrides it.",
          "Interviewers get an email reminder the day before. \"Add to calendar\" gives an .ics file for Outlook, Google or Apple Calendar.",
        ],
      },
      {
        heading: "Feedback",
        needs: ["interviews:feedback"],
        points: [
          "After the interview each interviewer gives strong yes, yes, no or strong no with comments.",
          "You see your colleagues' feedback only after giving your own, so the first opinion does not steer the rest.",
          "\"My upcoming interviews\" on the Dashboard lists the next two weeks.",
        ],
      },
      {
        heading: "The calendar",
        needs: ["calendars:use"],
        points: [
          "The calendar icon in the top bar opens it (day, week, month or agenda). The dock icon keeps it on the right while you work.",
          "Connect your Microsoft calendar to see busy times when scheduling.",
        ],
      },
      {
        heading: "Booking links",
        needs: ["scheduling:use"],
        points: [
          "Set your hours under Settings › Your account › Booking hours.",
          "In an email to a candidate, \"+ Booking link\" adds a link where they pick a time that suits them.",
        ],
      },
    ],
  },
  {
    id: "offers-placements",
    title: "Offers, placements and invoices",
    summary: "Make an offer, record the placement and its fee, and send the invoice.",
    words: "offer approve approval placement placed fee invoice billing guarantee replacement retainer paid overdue po terms of business sst",
    needs: ["offers:read", "placements:read"],
    links: [{ label: "Placements", href: "#/placements" }, { label: "Invoicing", href: "#/invoicing" }],
    sections: [
      {
        heading: "Offers",
        needs: ["offers:read"],
        points: [
          "On a candidate's job (from submitted on), Make offer drafts the salary, start date, the day to answer by and other terms.",
          "Recruiters ask for approval; owners, admins and hiring managers approve it or send it back. \"Offers to approve\" is on their Dashboard.",
          "Then mark it sent, and accepted or declined.",
        ],
      },
      {
        heading: "Placements",
        needs: ["placements:read"],
        points: [
          "At the offer stage, Record placement: the start date, guarantee period, the recruiter credited, the salary and the fee (a % of the yearly salary or a fixed amount).",
          "The fee starts from the client's terms of business (on the company's page) and can be changed for that placement.",
          "The Placements page shows starts, guarantees ending, invoices not sent and overdue, with the fees billed and paid.",
          "If someone leaves within the guarantee, mark it fell through: the client is owed a replacement until you mark it provided or refunded.",
        ],
      },
      {
        heading: "Invoices",
        needs: ["placements:read"],
        points: [
          "Settings › Invoicing holds your invoice template: your company's details, bank details, numbering, tax such as SST and the email that sends it.",
          "On the start date the invoice is numbered, made as a PDF and emailed to the client's contact. \"Send invoice now\" does it at once.",
          "Salaries and fees are seen only by roles allowed to see them.",
        ],
      },
    ],
  },
];
