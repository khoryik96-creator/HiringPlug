package jobs

// The jobs rules, as plain functions on plain values: no database, no HTTP.

import (
	"strconv"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Details are the editable fields of a job.
type Details struct {
	CompanyID       string
	ContactID       *string
	Title           string
	Status          string // open, on_hold, filled, closed
	Location        *string
	WorkType        *string
	WorkArrangement *string
	Openings        int
	Description     *string
	Country         *string   // ISO code, e.g. MY
	IntakeDate      time.Time // when the job was officially received (a date); zero on create = today
}

// SalaryRange is a job's salary budget: one figure (Max nil) or a range.
type SalaryRange struct {
	Min      string
	Max      *string
	Currency string
	Period   string
}

// Salary is the stored budget with its version.
type Salary struct {
	Version   int
	Range     *SalaryRange
	UpdatedAt time.Time
}

// JobOrder is one job.
type JobOrder struct {
	ID          string
	Version     int
	Details     Details
	CompanyName string
	ContactName *string
	OwnerID     *string // the recruiter it belongs to; nil for none
	OwnerName   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// View is a job as a user may see it: the salary only with permission.
type View struct {
	JobOrder
	Salary           *Salary // nil: none recorded (or restricted)
	SalaryRestricted bool
}

// Summary is a job in a list.
type Summary struct {
	ID, Title, CompanyID, CompanyName, Status string
	Location                                  *string
	Openings                                  int
	UpdatedAt                                 time.Time
}

// Patch is a partial update of Details.
type Patch struct {
	CompanyID       apiconv.Change[string]
	ContactID       apiconv.Change[*string]
	Title           apiconv.Change[string]
	Status          apiconv.Change[string]
	Location        apiconv.Change[*string]
	WorkType        apiconv.Change[*string]
	WorkArrangement apiconv.Change[*string]
	Openings        apiconv.Change[int]
	Description     apiconv.Change[*string]
	Country         apiconv.Change[*string]
	IntakeDate      apiconv.Change[time.Time]
}

// Apply returns d with the patch's fields replaced.
func (p Patch) Apply(d Details) Details {
	apiconv.Apply(&d.CompanyID, p.CompanyID)
	apiconv.Apply(&d.ContactID, p.ContactID)
	apiconv.Apply(&d.Title, p.Title)
	apiconv.Apply(&d.Status, p.Status)
	apiconv.Apply(&d.Location, p.Location)
	apiconv.Apply(&d.WorkType, p.WorkType)
	apiconv.Apply(&d.WorkArrangement, p.WorkArrangement)
	apiconv.Apply(&d.Openings, p.Openings)
	apiconv.Apply(&d.Description, p.Description)
	apiconv.Apply(&d.Country, p.Country)
	apiconv.Apply(&d.IntakeDate, p.IntakeDate)
	return d
}

// Normalize tidies a job's details and fills in defaults (status open, one
// opening), checking what the contract's schemas cannot.
func Normalize(d Details) (Details, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	d.Title = strings.Join(strings.Fields(d.Title), " ")
	if d.Title == "" {
		errs.Add("title", "is required")
	}
	if d.Status == "" {
		d.Status = "open"
	}
	if d.Openings == 0 {
		d.Openings = 1
	}
	if d.IntakeDate.IsZero() {
		d.IntakeDate = today()
	}
	if d.Location != nil {
		t := strings.Join(strings.Fields(*d.Location), " ")
		d.Location = &t
		if t == "" {
			d.Location = nil
		}
	}
	if d.Description != nil {
		t := strings.TrimSpace(*d.Description)
		d.Description = &t
		if t == "" {
			d.Description = nil
		}
	}
	return d, errs
}

// ValidateSalary checks a budget beyond the contract's schemas: the range
// must not run backwards.
func ValidateSalary(r *SalaryRange) apperr.FieldErrors {
	var errs apperr.FieldErrors
	if r == nil || r.Max == nil {
		return errs
	}
	lo, okLo := cents(r.Min)
	hi, okHi := cents(*r.Max)
	if okLo && okHi && hi < lo {
		errs.Add("range.max", "must not be less than min")
	}
	return errs
}

// cents converts "8500.5" to 850050 without floating point (the contract has
// already checked the format).
func cents(s string) (int64, bool) {
	whole, frac, _ := strings.Cut(s, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	return n, err == nil
}

// AdDetails are the editable fields of a job ad.
type AdDetails struct {
	Title           string
	Location        *string
	WorkType        *string
	WorkArrangement *string
	SalaryText      *string // published as written; not the job's private budget
	Description     *string
	ClosesOn        *time.Time // a date
	Status          string     // draft, published, closed
	Country         *string    // ISO code
	// ClientDescription is what the blind ad says instead of the client's
	// name, e.g. "a leading Malaysian bank".
	ClientDescription *string
	PublishBlind      bool // the careers page shows the blind version
}

// AdPatch is a partial update of AdDetails.
type AdPatch struct {
	Title             apiconv.Change[string]
	Location          apiconv.Change[*string]
	WorkType          apiconv.Change[*string]
	WorkArrangement   apiconv.Change[*string]
	SalaryText        apiconv.Change[*string]
	Description       apiconv.Change[*string]
	ClosesOn          apiconv.Change[*time.Time]
	Status            apiconv.Change[string]
	Country           apiconv.Change[*string]
	ClientDescription apiconv.Change[*string]
	PublishBlind      apiconv.Change[bool]
}

// Apply returns d with the patch's fields replaced.
func (p AdPatch) Apply(d AdDetails) AdDetails {
	apiconv.Apply(&d.Title, p.Title)
	apiconv.Apply(&d.Location, p.Location)
	apiconv.Apply(&d.WorkType, p.WorkType)
	apiconv.Apply(&d.WorkArrangement, p.WorkArrangement)
	apiconv.Apply(&d.SalaryText, p.SalaryText)
	apiconv.Apply(&d.Description, p.Description)
	apiconv.Apply(&d.ClosesOn, p.ClosesOn)
	apiconv.Apply(&d.Status, p.Status)
	apiconv.Apply(&d.Country, p.Country)
	apiconv.Apply(&d.ClientDescription, p.ClientDescription)
	apiconv.Apply(&d.PublishBlind, p.PublishBlind)
	return d
}

// NormalizeAd tidies an ad: spaces in one-line fields squeezed, the text
// trimmed (its line breaks kept), empty values cleared, new ads drafts.
func NormalizeAd(d AdDetails) (AdDetails, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	d.Title = strings.Join(strings.Fields(d.Title), " ")
	if d.Title == "" {
		errs.Add("title", "is required")
	}
	if d.Status == "" {
		d.Status = "draft"
	}
	d.Location, d.SalaryText = squeezedOrNil(d.Location), squeezedOrNil(d.SalaryText)
	d.ClientDescription = squeezedOrNil(d.ClientDescription)
	if d.Description != nil {
		t := strings.TrimSpace(*d.Description)
		d.Description = &t
		if t == "" {
			d.Description = nil
		}
	}
	return d, errs
}

func squeezedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.Join(strings.Fields(*s), " ")
	if t == "" {
		return nil
	}
	return &t
}

// PublishProblem says, in words, why an ad cannot be advertised as it is, or
// "" when it can. becoming is true when the ad is being published now (an ad
// already published stays up if its job is put on hold, until taken down).
func PublishProblem(d AdDetails, jobStatus string, today time.Time, becoming bool) string {
	switch {
	case becoming && jobStatus != "open":
		return "Only open jobs can be advertised; this job is " + strings.ReplaceAll(jobStatus, "_", " ") + "."
	case d.Description == nil:
		return "Write the ad's text before publishing it."
	case becoming && d.ClosesOn != nil && d.ClosesOn.Before(today):
		return "The closing date has passed; change it before publishing."
	}
	return ""
}
