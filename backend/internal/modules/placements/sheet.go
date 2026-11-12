package placements

// The placement sheet: the job title, client name and job type as the
// placement records them (filled in from the job, then the placement's own),
// the client's PO number and, when known, the day the placement ends.

import (
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Job types.
const (
	Permanent = "permanent"
	Contract  = "contract"
)

// Sheet is what the placement sheet says beyond the dates and guarantee.
type Sheet struct {
	JobTitle    *string // nil: the job's title
	CompanyName *string // nil: the client's name
	JobType     string  // permanent or contract; "": from the job
	PONumber    *string
	EndDate     *time.Time
}

// SheetPatch is a partial update of a Sheet.
type SheetPatch struct {
	JobTitle    apiconv.Change[*string]
	CompanyName apiconv.Change[*string]
	JobType     apiconv.Change[string]
	PONumber    apiconv.Change[*string]
	EndDate     apiconv.Change[*time.Time]
}

// Apply returns s with the patch's changes.
func (p SheetPatch) Apply(s Sheet) Sheet {
	apiconv.Apply(&s.JobTitle, p.JobTitle)
	apiconv.Apply(&s.CompanyName, p.CompanyName)
	apiconv.Apply(&s.JobType, p.JobType)
	apiconv.Apply(&s.PONumber, p.PONumber)
	apiconv.Apply(&s.EndDate, p.EndDate)
	return s
}

// JobTypeOf is the job type a job's work type means: contract and temporary
// jobs are contracts, anything else (or none) permanent.
func JobTypeOf(workType *string) string {
	if workType != nil && (*workType == "contract" || *workType == "temporary") {
		return Contract
	}
	return Permanent
}

// WithJob fills in what the sheet leaves out from the job.
func (s Sheet) WithJob(app Application) Sheet {
	if trimmed(s.JobTitle) == nil && app.JobTitle != "" {
		s.JobTitle = &app.JobTitle
	}
	if trimmed(s.CompanyName) == nil && app.CompanyName != "" {
		s.CompanyName = &app.CompanyName
	}
	if s.JobType == "" {
		s.JobType = JobTypeOf(app.WorkType)
	}
	return s
}

// OnSheet is the application as the placement sheet names it: its job title
// and client name.
func OnSheet(app Application, s Sheet) Application {
	if s.JobTitle != nil {
		app.JobTitle = *s.JobTitle
	}
	if s.CompanyName != nil {
		app.CompanyName = *s.CompanyName
	}
	return app
}

func (s Sheet) normalize(start time.Time, errs *apperr.FieldErrors) Sheet {
	s.JobTitle, s.CompanyName, s.PONumber = trimmed(s.JobTitle), trimmed(s.CompanyName), trimmed(s.PONumber)
	if s.JobType == "" {
		s.JobType = Permanent
	}
	if s.JobType != Permanent && s.JobType != Contract {
		errs.Add("jobType", "must be permanent or contract")
	}
	for _, f := range []struct {
		name string
		v    *string
		max  int
	}{{"jobTitle", s.JobTitle, 300}, {"companyName", s.CompanyName, 300}, {"poNumber", s.PONumber, 100}} {
		if f.v != nil && len([]rune(*f.v)) > f.max {
			errs.Add(f.name, "is too long")
		}
	}
	if s.EndDate != nil && s.EndDate.Before(start) {
		errs.Add("endDate", "is before the start date")
	}
	return s
}

func sheetAudit(s Sheet) map[string]any {
	var end any
	if s.EndDate != nil {
		end = s.EndDate.Format(time.DateOnly)
	}
	return map[string]any{"jobTitle": s.JobTitle, "companyName": s.CompanyName, "jobType": s.JobType, "poNumber": s.PONumber, "endDate": end}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
