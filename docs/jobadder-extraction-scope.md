# JobAdder full-account extraction scope

Audit date: 18 November 2026. Repository main reviewed: `985f000`.
This defines the required migration scope; it is not a statement that extraction
has already been implemented or that access to every item has been verified.

The requirement is all retained business data in the agency's JobAdder account,
including original fields, linked records, documents, communications and history.
No cleaning, salary recalculation, rewriting, automatic person/company merging,
reactivation, unarchiving or other JobAdder record changes are permitted.

## Current implementation

The published v2 specification contains 240 GET operations. Some are aliases,
lookup helpers or per-record routes; this is not 240 separate data types.
The tool bundles those routes in `tools/jobadder-migrator/extraction_routes.json`.
Its coverage report distinguishes actual queued/saved/unresolved requests from
bulk equivalents and routes awaiting source identifiers. Full-record/native-file
extraction and provider-original intake are implemented; discovery remains a
separate 47-resource count/definition workflow.

All live-account access remains unverified. Automated validation uses synthetic
records and responses. A finished API pass is not verified full-account coverage.
See [operation and format limits](../tools/jobadder-migrator/README.md).

## Required scope

| Area | Preserve and reconcile |
| --- | --- |
| Candidates | All retained statuses; names, contact methods, addresses, social links, summaries, employment/education history, original compensation and availability, ratings, seeking status, sources, recruiters, custom values and metadata. Include candidate/contact dual-role links. |
| Tags and coding | Free-text skill tags, structured skill/category assignments, job skill requirements, industry/category hierarchies and custom-field-based tags. Keep spelling, IDs, hierarchy, order and assignments; a tag definition alone is insufficient. |
| Notes and mentions | Every available note type/source: discussions, calls, emails/SMS logged as notes, subjects, full original text/formatting, author/editor IDs, timestamps, readonly flags, references, mentions as stored and every linked entity. Preserve note attachments and many-to-many links. |
| Emails | All retained recruitment conversations stored in JobAdder: sent/received messages, subject, original body, sender/recipients, CC/BCC where retained, dates, threading/message IDs, attachments and candidate/contact/company/job links. Prove full coverage independently of note counts. |
| SMS, chats and calls | Retained SMS, WhatsApp, call logs, voicemails, recordings and provider results, including direction, participants and links where available. Logged notes are one source, not proof that the complete provider history is present. |
| CVs and other files | Every available original/formatted CV version, cover letter, application file, job description, terms, contracts, references, checks, licences, certificates and other attachments. Include files on candidates, contacts, companies, jobs, applications, placements, requisitions and notes. |
| File metadata and media | Original file IDs, names, types/categories, uploaders, timestamps, expiry, record links and file bytes/checksums. Include photos, logos and videos; confirm whether APIs provide original media or only resized/latest/provider-linked versions. |
| Companies | Full records, all addresses, parent/subsidiary relationships, main/other contacts, ownership, recruiters, skills, custom values, notes, terms and documents. |
| Contacts | Full records, inactive contacts, company/reporting links, original/previous-contact links, owners, recruiters, custom values, communication preferences, notes and files. |
| Jobs, including archives | Open, closed, inactive and archived jobs retained by JobAdder; original descriptions, company/contact links, requirements, custom values, compensation, fees, owners/recruiters, close dates/authors and documents. Confirm the UI's archive semantics against source statuses. |
| Applications and pipeline | Active and inactive/rejected applications; original statuses, candidate/job/ad links, source, submitted details, screening questions/answers/scores, review stages/timestamps, ownership, notes, files and available historical transitions. |
| Client submissions and floats | Candidate/client/contact/job relationships, submission/marketing text, dates, review information and original sent document versions where retained. A link to today's CV does not recover the CV actually sent previously. |
| Placements | Permanent, contract, temporary and credit records; approved and unapproved records; retained cancelled/completed statuses, terms, dates, rates/currencies, billing, splits, recruiters, custom values, notes and files. |
| Invoices and commercial records | Retainers/invoice stages and data, expenses, fee splits, triggers, forecasts, sent details and placement/job links. Original invoice PDFs or emailed copies require their own file evidence; JSON invoice data is not the original PDF. |
| Timesheets and payroll | All retained periods, entries, submission/approval metadata, payroll/billing, awards, payment types, industry codes, rates and rate-card references, subject to enabled modules/access. Request full rate-card details if the API exposes only a reference. |
| Screening and activities | Activity types/statuses, form/question definitions, conditional questions, original answers and ratings for candidates, companies, contacts, jobs, applications and placements. Do not substitute an AI-generated summary for original answers. |
| Interviews and meetings | Participants, times/time zones, locations, statuses and links; internal and client feedback, evaluations/comments/ratings and relevant expiry/response metadata. Include old/cancelled records where retained. |
| Tasks and reminders | Old and completed as well as current tasks; subjects, descriptions, due/start dates, completion, assignees, ownership and linked entities. |
| Folders and shortlists | Folder definitions, owners, sharing and membership for every entity type. Include folders owned by inactive users and reconcile orphaned/deleted-user ownership through exports where necessary. |
| Job ads and boards | Ads in all retained states, original content, posting/expiry metadata, boards/portals, apply links, ownership, screening configuration and applications. Historical reposts/deletions require export checks where not exposed. |
| Requisitions and approvals | Forms, custom definitions/values, approvers, company workflows, approval history, notes, files and links to jobs. |
| Opportunities | Retained business-development records and stages, owners, linked companies, estimates/close dates, notes and all returned fields. |
| Users and organisation | Active/inactive/deleted user identity metadata, groups, offices, ownership, sharing and historical authorship. Importing an author must not automatically grant a HiringPlug login or elevated access. |
| Custom fields and lookups | Definitions AND per-record values, for contacts and requisitions as well as candidates, companies, jobs, applications and placements. Include statuses/workflows, sources, note types, attachment categories, ratings, salutations, locations, work types, billing/payment/award lists and event subtypes. |
| Privacy and visibility | Retained unsubscribe/communication flags, consent evidence and visibility/sharing information. Full consent/privacy history and permission definitions require export checks beyond the public API. Do not turn unknown visibility into public access. |
| Personal workspace data | Favourites, saved searches/filters, personal lists, dashboard layouts, report definitions and KPI/target settings. Some data is user-specific or absent from the public API; account-level access is not proof of all-user coverage. |
| Templates and settings | Retained email/SMS/job-ad templates, signatures, branding, forms, automation/sequence definitions and enrolment/history, and non-secret account configuration. Native HiringPlug behaviour may need separate implementation, but original data must remain archived. |
| Integration records | Accessible partner-action definitions, submitted-record links, statuses/results, webhook metadata and third-party assessment/compliance outputs. Extra scopes/provider exports may be required; no external link alone proves the referenced content was recovered. |
| Audit/history and deleted data | Every retained accessible audit/change history and deletion marker. Public recently-deleted routes cover only a limited window and do not return full historical record contents. Purged/non-retained data cannot be promised or recreated. |
| Extra/unsupported fields | Retain complete original responses, native exports and schema/relationship metadata. Any data without a native HiringPlug field needs a source-preserving archive/view, not silent omission. |

## Extraction traps confirmed against the public schema

1. **Notes:** `/notes` and entity note lists expose text as an additional field.
   Request `Fields=text` or use full-note detail responses; a note ID/subject
   inventory is not a note-body export. Save each note once by source ID and
   preserve its links to all associated candidates/companies/jobs/etc.
2. **Tags have several forms:** candidate `skillTags`, job skill tags, structured
   `/.../{id}/skills` coding, custom values and note mentions must be handled
   separately. Do not flatten them into one renamed tag list.
3. **Files:** candidate attachment lists support `Latest`. Explicitly inventory
   all available versions instead of limiting to latest. Binary attachment
   routes are separate from metadata; the candidate download operation can
   attempt conversion based on Accept. Use native-byte downloads, preserve the
   original file type and validate hashes. Do not reuse a JSON-only API client
   for native documents. Previously overwritten/deleted versions need backup checks.
4. **Archived jobs/applications:** account for `Active=false`, closed statuses,
   `closedAt` and rejected/inactive application partitions. Avoid `/active`
   shortcuts as the sole source. Reconcile IDs from filtered and unfiltered
   lists against account exports; ordinary default counts may omit data.
5. **Old tasks:** `Include` supports `Old` and `Complete`; current-task defaults
   are insufficient. Timesheet extraction must cover all relevant periods.
6. **Former users/folders:** user lists support `Include=Inactive` and `Deleted`;
   folder lists support active/inactive owner partitions. User-context favourites
   and private data require separate coverage evidence.
7. **Activity answers:** list operations can require `IncludeDetails`; fetch
   detailed activity records and their question/status definitions as necessary.
8. **Deleted records:** the six recently-deleted routes document the last
   **14 days**. Their models contain source IDs/deletion dates, not complete
   restorable records. Do not mark a tombstone as recovered record content.
9. **Email:** the published read API has an email-address matching helper, but
   no general email/thread/mailbox reader. Notes have subject/text/attachments;
   that does not establish complete email bodies, headers, threads or attachments.
10. **Integration access:** partner actions declare `partner_ui_action`; webhook
    reads declare `offline_access`. Broad record read access must not be treated
    as proof that all integration data is visible.
11. **Historical sent CVs:** submission/application details in this schema do
    not establish the exact historical CV binary sent to a client. Correlate
    retained communication/file snapshots or obtain them from the export.
12. **Rate cards:** the job rate-card response exposes a string reference.
    Do not call that a complete rate-card export.

## Required source strategy

Use API extraction plus a provider-supplied full account export/backup and its
native attachments/media manifest. Confirm the export's actual tables, files,
configuration, communication/history coverage, time boundaries and omissions.
JobAdder's export process is documented, but its inclusion of every requested
category and attachment must be confirmed for this account.

If recruitment messages remain only in connected Microsoft/Google or messaging
providers, obtain an appropriately scoped authorised export for those messages
and attachment relationships. That is a separate data source, not something
the existing JobAdder token automatically unlocks. No mailbox has been accessed
and no message has been sent during this audit.

Request logical relational exports with schema and original IDs/foreign keys
where possible. If a SQL Server backup is supplied, it requires a restore/read
adapter; the Go application cannot simply treat a `.bak` file as CSV or JSON.
Archive originals before projection into HiringPlug. Credentials/tokens must be
configured through secure connection flows, not copied as business records.

## Completion criteria

- Track every scope row and endpoint as verified exported, pending, denied,
  unsupported/API gap, not enabled, confirmed empty, or source-retained-data gap.
  Unknown/denied is never counted as zero and an omitted field is not an empty value.
- Reconcile source IDs by entity/status/time range with API inventories and the
  provider export. Counts alone are insufficient: verify relationships and
  boundary samples, including old/closed/inactive and former-user records.
- Reconcile file IDs, all available versions, bytes and checksums. Duplicate
  filenames must not overwrite one another; source filenames remain metadata
  even when safe storage paths differ. Keep every attachment-to-record link.
- Preserve original types, exact numeric values, dates/date precision, timestamps,
  names, labels, null/blank distinctions, rich text and unknown properties.
  Never regenerate missing notes, scores, messages or documents as originals.
- Preserve original IDs and authors separately from HiringPlug IDs. Deduplicate
  repeated retrieval of the SAME source ID, not different people/records sharing
  an email address, name, note text or file content.
- If JobAdder remains in use, do an initial extraction followed by a delta and
  final reconciliation. Backup/API observations are not one atomic snapshot.
- Extract unsupported data into a secure source archive and provide a planned
  legacy-data view in HiringPlug. Full extraction and native feature parity are
  separate acceptance checks; unsupported fields cannot simply disappear.
- List every unresolved omission for review. Do not label the migration complete
  while required scope is still unverified or silently excluded.

## Sources and evidence

- [JobAdder public v2 OpenAPI](https://api.jobadder.com/v2/openapi.json): live
  snapshot saved alongside this report; SHA-256 in `audit-summary.json`.
- [JobAdder API reference](https://api.jobadder.com/v2/docs).
- [JobAdder outreach](https://jobadder.com/platform/workspace/outreach): confirms
  messages from supported email/SMS/chat integrations can be logged to records;
  this does not promise a public full-history export endpoint.
- [JobAdder export process](https://jobadder.com/legal/standard-professional-service-terms),
  section 2, clause 14: request scope, timing and any applicable fees with the provider.
- [JobAdder account backup information](https://jobadder.com/legal/terms-of-use),
  clause 10(e): describes a requested SQL backup of live account data at termination;
  it is not a guarantee that every historical file or message is included.

No code changes or live extraction were performed. This audit expands the
required implementation scope; it does not upgrade the merged discovery-only tool.
