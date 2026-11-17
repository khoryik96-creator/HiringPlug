package main

// Only audited GET endpoints are probed. Nested/file/history coverage is reported
// separately: a collection count is not evidence that its children were exported.
type Resource struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Path  string `json:"path"`
	Kind  string `json:"kind"`
}

func Catalog() []Resource {
	var out []Resource
	for _, r := range []struct{ key, label, path string }{
		{"candidates", "Candidates", "/candidates"}, {"companies", "Companies", "/companies"},
		{"contacts", "Contacts", "/contacts"}, {"jobs", "Jobs", "/jobs"},
		{"applications", "Job applications", "/applications"}, {"placements", "Placements", "/placements"},
		{"notes", "Notes", "/notes"}, {"submissions", "Submissions", "/submissions"},
		{"floats", "Candidate floats", "/floats"}, {"interviews", "Interviews", "/interviews"},
		{"events", "Events", "/events"}, {"tasks", "User tasks", "/usertasks"},
		{"folders", "Folders", "/folders"}, {"users", "Users", "/users"},
		{"usergroups", "User groups", "/usergroups"}, {"offices", "User offices", "/useroffices"},
		{"jobads", "Job ads", "/jobads"}, {"opportunities", "Opportunities", "/opportunities"},
		{"requisitions", "Requisitions", "/requisitions"},
		{"candidate_attachments", "Candidate attachment metadata", "/candidates/attachments"},
	} {
		out = append(out, Resource{r.key, r.label, r.path, "count"})
	}
	// This FT360 lookup accepts candidate IDs, not Limit=0 or a global total.
	out = append(out, Resource{"payroll", "Candidate payroll/billing", "/candidates/payrollbilling", "deferred"})
	for _, entity := range []string{"candidates", "companies", "contacts", "jobs", "applications", "placements"} {
		out = append(out, Resource{entity + "_activities", entity + " activities", "/" + entity + "/activities", "count"})
	}
	for _, entity := range []string{"candidates", "companies", "jobs", "applications", "placements"} {
		out = append(out, Resource{entity + "_custom", entity + " custom-field definitions", "/" + entity + "/fields/custom", "definitions"})
	}
	for _, entity := range []string{"candidates", "companies", "contacts", "jobs", "placements"} {
		out = append(out, Resource{entity + "_statuses", entity + " status definitions", "/" + entity + "/lists/status", "definitions"})
	}
	for _, entity := range []string{"candidates", "companies", "jobs", "placements"} {
		out = append(out, Resource{entity + "_attachment_categories", entity + " attachment categories", "/" + entity + "/lists/attachmentcategory", "definitions"})
	}
	for _, r := range []Resource{
		{"candidate_sources", "Candidate sources", "/candidates/lists/source", "definitions"},
		{"job_sources", "Job sources", "/jobs/lists/source", "definitions"},
		{"categories", "Categories", "/categories", "paged_definitions"},
		{"countries", "Countries", "/countries", "definitions"},
		{"locations", "Locations", "/locations", "definitions"},
		{"worktypes", "Work types", "/worktypes", "definitions"},
	} {
		out = append(out, r)
	}
	return out
}

var Coverage = []string{
	"This is read-only discovery, not a completed migration or a full account backup.",
	"Counts reflect the connected user's API permissions and JobAdder's default list visibility; reconcile against account exports.",
	"CV/file bytes and versions, folder memberships, company/contact/job/application/placement attachments, invoices, timesheets, rate cards, requisition forms/approval history and other per-record relationships still require deeper inventory.",
	"Payroll/billing requires candidate-level discovery and is not requested during this count-only inventory; access and coverage have not been verified.",
	"Email/SMS bodies, complete stage histories, deleted/purged records and any data not exposed to this account require separate access/export checks. Unavailable is never reported as zero.",
	"Original JSON responses are retained byte for byte. No source fields are mapped, cleaned, merged or sent to AI or HiringPlug.",
	"Known unpaged definition envelopes are counted from their items. Unexpected pagination requires review; record-level custom-field values are not exported by discovery.",
	"Inventory requests occur at different times; changing counts are not a transactionally consistent JobAdder snapshot.",
}
