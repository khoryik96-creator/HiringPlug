package careers

// The careers rules, as plain functions on plain values: no database, no HTTP.

import (
	"net/mail"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Form is what the apply form sends, besides the CV.
type Form struct {
	FirstName, LastName, Email, Phone string
	Consent                           string // "yes"
	Website                           string // a trap for robots: people never see it, so it stays empty
}

// IsRobot reports a form that filled in the hidden field. It is answered as if
// it worked, so robots learn nothing.
func (f Form) IsRobot() bool {
	return strings.TrimSpace(f.Website) != ""
}

// Validate tidies the form and checks what the contract cannot.
func (f Form) Validate() (Applicant, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	a := Applicant{
		FirstName: strings.Join(strings.Fields(f.FirstName), " "),
		LastName:  strings.Join(strings.Fields(f.LastName), " "),
		Email:     strings.ToLower(strings.TrimSpace(f.Email)),
	}
	if a.FirstName == "" {
		errs.Add("firstName", "is required")
	}
	if len(a.FirstName) > 100 || len(a.LastName) > 100 {
		errs.Add("lastName", "names can be at most 100 characters")
	}
	if addr, err := mail.ParseAddress(a.Email); err != nil || addr.Address != a.Email || !strings.Contains(a.Email[strings.LastIndex(a.Email, "@")+1:], ".") {
		errs.Add("email", "is not a valid email address")
	}
	if phone := strings.TrimSpace(f.Phone); phone != "" {
		a.Phone = &phone
	}
	if f.Consent != "yes" {
		errs.Add("consent", "tick the box to agree before applying")
	}
	return a, errs
}
