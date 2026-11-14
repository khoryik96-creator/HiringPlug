package contacts

// The contacts rules, as plain functions on plain values: no database, no HTTP.

import (
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/linkedin"
)

// Details are the editable fields of a contact.
type Details struct {
	CompanyID   string
	FirstName   string
	LastName    string
	Title       *string
	Email       *string
	Phone       *string
	Mobile      *string
	LinkedinURL *string
	Status      string  // active or left
	Role        *string // a key of the contact_role list (hiring_manager, hr…); nil: not set
	Notes       *string
}

// EmailCheckResult is the outcome of checking an email address.
type EmailCheckResult struct {
	Status    string
	Reasons   []string
	CheckedAt time.Time
}

// Contact is one contact.
type Contact struct {
	ID          string
	Version     int
	CompanyName string
	Details     Details
	EmailCheck  *EmailCheckResult // nil without an email
	Employment  *EmploymentCheck  // the latest LinkedIn check; nil before the first
	Mailbox     *MailboxCheck     // the latest mailbox check of Details.Email; nil before the first
	Linkedin    *LinkedinCheck    // the latest answered LinkedIn check; nil before the first
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Summary is a contact in a list.
type Summary struct {
	ID, CompanyID, CompanyName, FirstName, LastName, Status            string
	Title, Email, EmailCheckStatus, MailboxStatus, Phone, Mobile, Role *string
	UpdatedAt                                                          time.Time
	Linkedin                                                           *LinkedinEmployment // nil before the first LinkedIn check
	MailboxCheckedAt                                                   *time.Time
}

// LinkedinEmployment is what the latest LinkedIn check saw.
type LinkedinEmployment struct {
	Current      *bool
	Joined, Left *string
	CheckedAt    time.Time
}

// Patch is a partial update of Details.
type Patch struct {
	CompanyID   apiconv.Change[string]
	FirstName   apiconv.Change[string]
	LastName    apiconv.Change[string]
	Title       apiconv.Change[*string]
	Email       apiconv.Change[*string]
	Phone       apiconv.Change[*string]
	Mobile      apiconv.Change[*string]
	LinkedinURL apiconv.Change[*string]
	Status      apiconv.Change[string]
	Role        apiconv.Change[*string]
	Notes       apiconv.Change[*string]
}

// Apply returns d with the patch's fields replaced.
func (p Patch) Apply(d Details) Details {
	apiconv.Apply(&d.CompanyID, p.CompanyID)
	apiconv.Apply(&d.FirstName, p.FirstName)
	apiconv.Apply(&d.LastName, p.LastName)
	apiconv.Apply(&d.Title, p.Title)
	apiconv.Apply(&d.Email, p.Email)
	apiconv.Apply(&d.Phone, p.Phone)
	apiconv.Apply(&d.Mobile, p.Mobile)
	apiconv.Apply(&d.LinkedinURL, p.LinkedinURL)
	apiconv.Apply(&d.Status, p.Status)
	apiconv.Apply(&d.Role, p.Role)
	apiconv.Apply(&d.Notes, p.Notes)
	return d
}

// FullName joins first and last name.
func FullName(first, last string) string {
	return strings.TrimSpace(first + " " + last)
}

// Normalize tidies a contact's details (spaces, lower-case email, LinkedIn
// address) and checks what the contract's schemas cannot.
func Normalize(d Details) (Details, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	d.FirstName = squeeze(d.FirstName)
	d.LastName = squeeze(d.LastName)
	if d.FirstName == "" {
		errs.Add("firstName", "is required")
	}
	if d.Status == "" {
		d.Status = "active"
	}
	for _, f := range []**string{&d.Title, &d.Phone, &d.Mobile, &d.Role} {
		*f = squeezedOrNil(*f)
	}
	if n := d.Notes; n != nil {
		t := strings.TrimSpace(*n)
		d.Notes = &t
		if t == "" {
			d.Notes = nil
		}
	}
	if e := squeezedOrNil(d.Email); e != nil {
		lower := strings.ToLower(*e)
		d.Email = &lower
		if !WellFormedEmail(lower) {
			errs.Add("email", "is not a valid email address")
		}
	} else {
		d.Email = nil
	}
	if l := squeezedOrNil(d.LinkedinURL); l != nil {
		normal, ok := NormalizeLinkedIn(*l)
		if !ok {
			errs.Add("linkedinUrl", "is not a LinkedIn address, e.g. linkedin.com/in/name")
		}
		d.LinkedinURL = &normal
	} else {
		d.LinkedinURL = nil
	}
	return d, errs
}

// NormalizeLinkedIn turns "linkedin.com/in/siti" into "https://www.linkedin.com/in/siti".
func NormalizeLinkedIn(raw string) (string, bool) { return linkedin.Normalize(raw) }

func squeeze(s string) string { return strings.Join(strings.Fields(s), " ") }

func squeezedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := squeeze(*s)
	if t == "" {
		return nil
	}
	return &t
}
