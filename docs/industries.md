# Industries and subindustries

The agency catalogue starts with the uploaded CV Studio/Kano vocabulary: 10
industries and 81 subindustries. Original labels and source keys are retained;
these source keys are not live JobAdder API identifiers. Education and
Government Sector have no subindustries. The seed is in the Go industries
module, as the authoritative full vocabulary; old broad-list defaults serve only as
compatibility fallbacks.

## Selection and recognition

Candidates and companies use the same searchable, two-column hierarchy.
Candidate selections are editable in profile editing and screening calls, and
can be included as named industries/subindustries in selected-candidate emails.
Recruiters can select several industries/subindustries or an industry alone.
One selection is current; earlier selections stay. Specialization is a
separate job-function field. Advanced candidate search matches both current
and historical tags; selecting a parent also matches its children. Retired
selections remain readable and searchable.

Go selects an unambiguous current employer, otherwise the latest ended job by
date. Concurrent employers, tied dates and incomplete evidence expose an
employer choice. An unrelated undated historical job does not invalidate a
clear current employer. The employer's business wins over its employee's
job title: SAP at a bank is Banking; SAP at a verified hardware company stays
Hardware. Enterprise-application role rules require a verified IT employer.
A customer's business is not substituted for the employing business.

Employer matching to saved companies uses exact normalized names. A company's
current selection is trusted agency evidence. Narrow employer-name rules
provide additional suggestions. Unknown employers stay open for manual
selection; they do not disable saving. Suggestions include candidate and
catalogue versions and require an explicit action before changing selections.
There is no automatic removal of existing tags and no paid classification call.

After CV extraction/grounding, Go selects the employer and supplies the broad
industry review suggestion. Python extracts evidence and fills missing contact
details and undated employer evidence; it no longer classifies industries, chooses a job by list position or
spends tokens on an industry-only request. Older parsed documents without a
selected-employer index require an unambiguous current job in CV review.

## Settings and safe edits

Settings > Industries allows owners/admins (settings:manage) to add, rename,
retire, restore, move and delete entries, and edit search aliases. Labels and
aliases use normalized case, Unicode dashes and spacing for matching; ambiguous
aliases are not guessed. Category IDs remain stable when names change.

The hierarchy has at most two levels. An industry with children cannot become
a child. Moving a subindustry updates its existing records' grouping. Retiring
a parent retires its children; restore each child explicitly when required.
Unused categories can be deleted. Used categories require retirement or an
explicit replacement outside the removed group; parent deletion includes its
children. Reassignment preserves other tags and merges duplicate assignments.
Every mutation checks the catalogue version and rechecks usage in the same
transaction. A changed catalogue asks an open editor to review its selections.

## Ownership and compatibility

`industries` owns catalogue and assignment tables, canonical matching and
recognition. Keeping assignments with the catalogue makes replacement and
deletion atomic without cross-module SQL. Candidates/companies still own their
records; they validate access, call catalogue interfaces inside their save
transaction and retain optimistic record versions. App adapters are the only
place modules meet. Duplicate merging transfers classifications, and foreign
keys remove them when a record is deleted.

All tables have tenant RLS. Catalogue reads expose usage counts only to Settings
managers. Stable IDs, valid parents, active choices and catalogue/record versions
are checked by Go. Catalogue mutations and record edits take the catalogue lock
before record writes; ordinary reads do not serialize all agency requests.

The migration seeds existing agencies and preserves legacy selections and custom
options independently of the first user's visibility. Unknown historical labels
remain retired entries. New agencies are seeded once on first use. Older clients can continue importing
unlisted scalar values as marked legacy entries; these are not offered as new
choices in the picker. Existing
scalar industry APIs and summaries remain broad-industry compatibility views;
renames and moves update summaries and record versions. The old flat Settings
industry endpoint is read-only: edits use the catalogue API. Optional
`industrySelections` in candidate/company create/update requests saves the full
selection atomically; omitted selections preserve older API-client support.
