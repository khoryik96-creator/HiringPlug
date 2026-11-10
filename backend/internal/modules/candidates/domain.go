package candidates

// The candidate rules, as plain functions on plain values: no database, no
// HTTP. Everything here is unit-tested in domain_test.go.

import (
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nyaruka/phonenumbers"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/linkedin"
)

// Work authorization statuses and bases (mirroring the contract's enums).
const (
	WorkAuthLocal         = "local"
	WorkAuthExpatEligible = "expat_eligible"
	WorkAuthRequiresVisa  = "requires_visa"
)

// Profile is everything about a candidate except salary details.
type Profile struct {
	FirstName         string
	LastName          string
	Email             *string
	Phone             *string
	Nationality       *string
	ResidenceCountry  *string // ISO code; nil = not known (NA)
	LinkedinURL       *string // https://www.linkedin.com/in/...
	Availability      *string // looking, passive, not_looking, placed; nil = not set
	WorkAuthorization *WorkAuthorization
	NoticePeriod      *NoticePeriod
	Industry          *string
	Specialization    *string
	CurrentTitle      *string
	CurrentCompany    *string
	Location          *string
	Source            *string
	Tags              []string // your own labels, e.g. relocate
	Skills            []string
}

// WorkAuthorization says whether the candidate can work here, and why.
type WorkAuthorization struct {
	Status string  // local | expat_eligible | requires_visa
	Basis  *string // rp_t | permanent_resident | spouse_visa | other; only for expat_eligible
	Note   *string
}

// NoticePeriod is how long the candidate must wait before starting. Value 0
// means immediately.
type NoticePeriod struct {
	Value int
	Unit  string // day | week | month
}

// Candidate is a stored candidate.
type Candidate struct {
	ID      string
	Version int
	Profile Profile
	OwnerID *string // the recruiter they belong to; nil for none
	// AvailabilitySince is when the status (Profile.Availability) was last set.
	AvailabilitySince *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// View is a candidate as one particular user may see it.
type View struct {
	Candidate
	History      History
	Compensation *Compensation // nil when none is recorded or the user may not see it
	// CompensationRestricted is true when the user lacks permission to see
	// salary details; the API then omits them and says so.
	CompensationRestricted bool
	OwnerName              *string
}

// Compensation holds salary details. Amounts are exact decimal strings.
type Compensation struct {
	Version   int
	Current   *SalaryAmount
	Expected  *SalaryRange
	UpdatedAt time.Time
}

// SalaryAmount is one figure, e.g. 8500.00 MYR per month.
type SalaryAmount struct {
	Description *string
	Amount      string
	Currency    string
	Period      string // hour | day | month | year
}

// SalaryRange is an expected salary: Min alone, or Min to Max.
type SalaryRange struct {
	Description *string
	Min         string
	Max         *string
	Currency    string
	Period      string
}

// Change is one field of a partial update. Set is false when the client did
// not send the field (leave it alone); for pointer types a nil Value clears it.
type Change[T any] struct {
	Set   bool
	Value T
}

// ProfilePatch is a partial update of a Profile.
type ProfilePatch struct {
	FirstName         Change[string]
	LastName          Change[string]
	Email             Change[*string]
	Phone             Change[*string]
	Nationality       Change[*string]
	ResidenceCountry  Change[*string]
	LinkedinURL       Change[*string]
	Availability      Change[*string]
	WorkAuthorization Change[*WorkAuthorization]
	NoticePeriod      Change[*NoticePeriod]
	Industry          Change[*string]
	Specialization    Change[*string]
	CurrentTitle      Change[*string]
	CurrentCompany    Change[*string]
	Location          Change[*string]
	Source            Change[*string]
	Tags              Change[[]string]
	Skills            Change[[]string]
}

// Apply returns p with the patch's fields replaced.
func (pp ProfilePatch) Apply(p Profile) Profile {
	apply(&p.FirstName, pp.FirstName)
	apply(&p.LastName, pp.LastName)
	apply(&p.Email, pp.Email)
	apply(&p.Phone, pp.Phone)
	apply(&p.Nationality, pp.Nationality)
	apply(&p.ResidenceCountry, pp.ResidenceCountry)
	apply(&p.LinkedinURL, pp.LinkedinURL)
	apply(&p.Availability, pp.Availability)
	apply(&p.WorkAuthorization, pp.WorkAuthorization)
	apply(&p.NoticePeriod, pp.NoticePeriod)
	apply(&p.Industry, pp.Industry)
	apply(&p.Specialization, pp.Specialization)
	apply(&p.CurrentTitle, pp.CurrentTitle)
	apply(&p.CurrentCompany, pp.CurrentCompany)
	apply(&p.Location, pp.Location)
	apply(&p.Source, pp.Source)
	apply(&p.Tags, pp.Tags)
	apply(&p.Skills, pp.Skills)
	return p
}

func apply[T any](field *T, c Change[T]) {
	if c.Set {
		*field = c.Value
	}
}

// NormalizeProfile tidies a profile (trims spaces, lower-cases the email,
// formats the phone number internationally) and checks the rules that the
// contract's schemas cannot express. phoneRegion is the country assumed for
// numbers written without a country code, e.g. "MY".
func NormalizeProfile(p Profile, phoneRegion string) (Profile, apperr.FieldErrors) {
	var errs apperr.FieldErrors

	p.FirstName = squeeze(p.FirstName)
	p.LastName = squeeze(p.LastName)
	if p.FirstName == "" {
		errs.Add("firstName", "is required")
	}

	p.Email = trimmedOrNil(p.Email)
	if p.Email != nil {
		email := strings.ToLower(*p.Email)
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			errs.Add("email", "is not a valid email address")
		}
		p.Email = &email
	}

	p.Phone = trimmedOrNil(p.Phone)
	if p.Phone != nil {
		e164, ok := NormalizePhone(*p.Phone, phoneRegion)
		if !ok {
			errs.Add("phone", "is not a valid phone number; include the country code, e.g. +60 12 345 6789")
		} else {
			p.Phone = &e164
		}
	}

	if l := trimmedOrNil(p.LinkedinURL); l != nil {
		normal, ok := linkedin.Normalize(*l)
		if !ok {
			errs.Add("linkedinUrl", "is not a LinkedIn profile address, e.g. linkedin.com/in/name")
		}
		p.LinkedinURL = &normal
	} else {
		p.LinkedinURL = nil
	}
	if p.Nationality != nil {
		n := strings.ToUpper(strings.TrimSpace(*p.Nationality))
		p.Nationality = &n
	}

	if wa := p.WorkAuthorization; wa != nil {
		wa.Note = trimmedOrNil(wa.Note)
		switch {
		case wa.Status == WorkAuthExpatEligible && wa.Basis == nil:
			errs.Add("workAuthorization.basis", "is required for expat_eligible (rp_t, permanent_resident, spouse_visa or other)")
		case wa.Status != WorkAuthExpatEligible && wa.Basis != nil:
			errs.Add("workAuthorization.basis", "only applies when status is expat_eligible")
		}
	}

	for _, f := range []**string{&p.Industry, &p.Specialization, &p.CurrentTitle, &p.CurrentCompany, &p.Location} {
		*f = trimmedOrNil(*f)
	}
	p.Tags = CleanLabels(p.Tags)
	p.Skills = CleanLabels(p.Skills)
	checkLengths(p, &errs)
	return p, errs
}

// checkLengths applies the contract's (and the database's) limits, for
// profiles that do not come through the API, such as one read from a CV.
func checkLengths(p Profile, errs *apperr.FieldErrors) {
	long := func(field string, v *string, maxRunes int) {
		if v != nil && utf8.RuneCountInString(*v) > maxRunes {
			errs.Add(field, fmt.Sprintf("must be at most %d characters", maxRunes))
		}
	}
	long("firstName", &p.FirstName, 100)
	long("lastName", &p.LastName, 100)
	long("email", p.Email, 254)
	long("linkedinUrl", p.LinkedinURL, 300)
	long("industry", p.Industry, 100)
	long("specialization", p.Specialization, 100)
	long("currentTitle", p.CurrentTitle, 200)
	long("currentCompany", p.CurrentCompany, 200)
	long("location", p.Location, 200)
	if len(p.Skills) > 100 {
		errs.Add("skills", "must be at most 100")
	}
	if len(p.Tags) > 50 {
		errs.Add("tags", "must be at most 50")
	}
	for i, s := range p.Skills {
		long(fmt.Sprintf("skills[%d]", i), &s, 60)
	}
	for i, t := range p.Tags {
		long(fmt.Sprintf("tags[%d]", i), &t, 60)
	}
}

// NormalizePhone returns number in E.164 form (+60123456789), or false if it
// is not a valid phone number.
func NormalizePhone(number, region string) (string, bool) {
	parsed, err := phonenumbers.Parse(number, region)
	if err != nil || !phonenumbers.IsValidNumber(parsed) {
		return "", false
	}
	return phonenumbers.Format(parsed, phonenumbers.E164), true
}

// FullName joins first and last name.
func FullName(p Profile) string {
	return strings.TrimSpace(p.FirstName + " " + p.LastName)
}

// ValidateCompensation checks salary details beyond what the contract's
// schemas check: an expected range must not run backwards.
func ValidateCompensation(current *SalaryAmount, expected *SalaryRange) apperr.FieldErrors {
	var errs apperr.FieldErrors
	if current != nil {
		if _, ok := Cents(current.Amount); !ok {
			errs.Add("currentSalary.amount", "must be an amount like 8500 or 8500.50")
		}
	}
	if expected != nil {
		lo, okMin := Cents(expected.Min)
		if !okMin {
			errs.Add("expectedSalary.min", "must be an amount like 8500 or 8500.50")
		}
		if expected.Max != nil {
			hi, okMax := Cents(*expected.Max)
			switch {
			case !okMax:
				errs.Add("expectedSalary.max", "must be an amount like 9000 or 9000.50")
			case okMin && hi < lo:
				errs.Add("expectedSalary.max", "must not be less than min")
			}
		}
	}
	return errs
}

// Cents converts a decimal string like "8500.5" to 850050 without floating
// point. It accepts up to 12 digits and 2 decimal places.
func Cents(s string) (int64, bool) {
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" || len(whole) > 12 || (hasFrac && (frac == "" || len(frac) > 2)) {
		return 0, false
	}
	for len(frac) < 2 {
		frac += "0"
	}
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil || n < 0 || strings.ContainsAny(whole+frac, "+-") {
		return 0, false
	}
	return n, true
}

// squeeze trims and collapses runs of whitespace to one space.
func squeeze(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// linesOrNil tidies text kept line by line (a job's duties): spaces squeezed
// in each line, blank lines between them at most one, none at the ends.
func linesOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(*s, "\r", ""), "\n") {
		line = squeeze(line)
		if line == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}
	t := strings.TrimSpace(strings.Join(out, "\n"))
	if t == "" {
		return nil
	}
	return &t
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := squeeze(*s)
	if t == "" {
		return nil
	}
	return &t
}

// CleanLabels tidies tags or skills: spaces squeezed, empty ones dropped, and
// duplicates ignoring case removed (the first spelling is kept). Never nil.
func CleanLabels(labels []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, l := range labels {
		l = squeeze(l)
		key := strings.ToLower(l)
		if l == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, l)
	}
	return out
}

// Job is one entry of a candidate's work history.
type Job struct {
	Title, Company     *string
	StartDate, EndDate *string // YYYY-MM or YYYY
	Current            bool
	Description        *string
}

// Qualification is one entry of a candidate's education.
type Qualification struct {
	Qualification, Institution *string
	EndYear                    *int
}

// History is a candidate's work history and education, newest first.
type History struct {
	Work      []Job
	Education []Qualification
}

// NormalizeHistory tidies text and checks each entry: a job needs a title or
// a company, dates in order, and no end date while current; a qualification
// needs a name or an institution.
func NormalizeHistory(h History) (History, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	out := History{Work: make([]Job, len(h.Work)), Education: make([]Qualification, len(h.Education))}
	for i, j := range h.Work {
		at := "experience[" + strconv.Itoa(i) + "]"
		j.Title, j.Company, j.Description = trimmedOrNil(j.Title), trimmedOrNil(j.Company), linesOrNil(j.Description)
		j.StartDate, j.EndDate = trimmedOrNil(j.StartDate), trimmedOrNil(j.EndDate)
		if j.Title == nil && j.Company == nil {
			errs.Add(at+".title", "enter a job title or a company")
		}
		for field, d := range map[string]*string{"startDate": j.StartDate, "endDate": j.EndDate} {
			if d != nil && !validYearMonth(*d) {
				errs.Add(at+"."+field, "use YYYY-MM or YYYY")
			}
		}
		if j.Current && j.EndDate != nil {
			errs.Add(at+".endDate", "leave the end date empty for a current job")
		}
		if j.StartDate != nil && j.EndDate != nil && (*j.EndDate)[:4] < (*j.StartDate)[:4] ||
			j.StartDate != nil && j.EndDate != nil && len(*j.StartDate) == 7 && len(*j.EndDate) == 7 && *j.EndDate < *j.StartDate {
			errs.Add(at+".endDate", "is before the start date")
		}
		out.Work[i] = j
	}
	for i, q := range h.Education {
		q.Qualification, q.Institution = trimmedOrNil(q.Qualification), trimmedOrNil(q.Institution)
		if q.Qualification == nil && q.Institution == nil {
			errs.Add("education["+strconv.Itoa(i)+"].qualification", "enter a qualification or an institution")
		}
		out.Education[i] = q
	}
	return out, errs
}

func validYearMonth(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	year, err := strconv.Atoi(s[:4])
	if err != nil || year < 1950 || year > 2100 {
		return false
	}
	if len(s) == 4 {
		return true
	}
	month, err := strconv.Atoi(s[5:])
	return err == nil && s[4] == '-' && month >= 1 && month <= 12
}
