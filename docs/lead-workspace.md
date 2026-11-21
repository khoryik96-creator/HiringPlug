# Leads workspace

A lead represents a job ad. A recruiter can browse the agency's leads or
find possible jobs from a candidate's CV, open the same lead for review or
star it for themselves. Agency lead status, company association and notes
are shared. Favorites belong to the individual recruiter.

## First release

- Searches use active feeds configured in Leads, selected countries and the
  title family inferred from the chosen original CV. Recruiters can enter a
  target title for a career change. The CV text stays inside HiringPlug.
- The search uses Greenhouse, Lever, Ashby and SmartRecruiters feed records.
  A result must have an individual ad URL matching the opening's provider
  ID. Generic pages, unsupported URL formats, closed leads and leads without
  matching location evidence stay out of discovery. Matching titles are
  sorted newest first. No fit percentages are presented.
- The existing Leads list continues to support manually saved ads, careers
  pages, source management, company links and lead status.
- Job descriptions received with a feed are stored as text and read from the
  detail endpoint. An update without a description preserves the earlier
  observation. Each description has its own last-observed date.
- A lead linked to an ATS company can open that company's Find HR tab. It
  uses the existing LinkedIn companion, employment evidence and contact
  review. Outreach uses the existing HiringPlug email and Outlook flows.

## Source and ATS map

| CV Studio reference | HiringPlug owner |
| --- | --- |
| CV selection and extraction | Documents module, bound to the selected candidate and document ID |
| Role family, title and region matching from Lead Finder | Deterministic Go rules, indexed lead queries, TypeScript filters |
| Direct ad recognition and source observations | Go adapter for each job feed, versioned lead detail evidence |
| Saved Job Leads and review | Existing Go leads module, Postgres, cursor-paged React workspace |
| HR scan and employment evidence | Contacts module and the existing LinkedIn companion, scoped to a linked ATS company |
| Outlook draft and send | Existing email module and composer, with explicit review and send |

## Adding external sources

General web discovery needs a configured search provider. Its next stage
uses server-side credentials, role and region queries without raw CV text,
provider quotas and separate adapters for links and ad availability. Keep
search results scoped to a recruiter and candidate/document revision. Persist
per-source progress and pages so Stop and targeted retries retain completed
results. Use source-backed posting identity and observations to merge an ad
with its existing HiringPlug lead without overwriting recruiter notes.

LinkedIn companion job discovery must reuse the approved ATS origin and
validate each run, page and observed identity. Preserve the public-feed
policy of the current Leads module. Extend to other providers only when
their authorized interfaces and evidence are understood. Apollo reveal and
JobAdder contact writes need their own explicit review and durable receipts.
They are outside this release.

## Update and rollback

Migration 00156 creates personal favorites, separate description storage and
search indexes. Existing leads and feed settings are preserved. Take the
ordinary database backup before an update. The previous application can run
with the additive schema in place. Keep that schema when rolling back the
application so favorites and observations survive a later upgrade.
