package companies

// The companies rules, as plain functions on plain values: no database, no HTTP.

import (
	"net/url"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Details are the editable fields of a company.
type Details struct {
	Name               string
	Type               string  // client, prospect or internal
	Category           *string // a key of the company_category list; nil: not set (Unknown)
	Website            *string
	Industry           *string
	Size               *string
	Phone              *string
	Address            *string
	City               *string
	Country            *string
	RegistrationNumber *string
	About              *string
}

// Company is one company record.
type Company struct {
	ID        string
	Version   int
	Details   Details
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Summary is a company in a list.
type Summary struct {
	ID        string
	Name      string
	Type      string
	Category  *string
	Industry  *string
	City      *string
	Website   *string
	UpdatedAt time.Time
}

// Patch is a partial update of Details.
type Patch struct {
	Name               apiconv.Change[string]
	Type               apiconv.Change[string]
	Category           apiconv.Change[*string]
	Website            apiconv.Change[*string]
	Industry           apiconv.Change[*string]
	Size               apiconv.Change[*string]
	Phone              apiconv.Change[*string]
	Address            apiconv.Change[*string]
	City               apiconv.Change[*string]
	Country            apiconv.Change[*string]
	RegistrationNumber apiconv.Change[*string]
	About              apiconv.Change[*string]
}

// Apply returns d with the patch's fields replaced.
func (p Patch) Apply(d Details) Details {
	apiconv.Apply(&d.Name, p.Name)
	apiconv.Apply(&d.Type, p.Type)
	apiconv.Apply(&d.Category, p.Category)
	apiconv.Apply(&d.Website, p.Website)
	apiconv.Apply(&d.Industry, p.Industry)
	apiconv.Apply(&d.Size, p.Size)
	apiconv.Apply(&d.Phone, p.Phone)
	apiconv.Apply(&d.Address, p.Address)
	apiconv.Apply(&d.City, p.City)
	apiconv.Apply(&d.Country, p.Country)
	apiconv.Apply(&d.RegistrationNumber, p.RegistrationNumber)
	apiconv.Apply(&d.About, p.About)
	return d
}

// DefaultType is the type of a company added without one.
const DefaultType = "client"

// Normalize tidies a company's details (spaces, website address, country
// code) and checks what the contract's schemas cannot.
func Normalize(d Details) (Details, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	d.Name = strings.Join(strings.Fields(d.Name), " ")
	if d.Name == "" {
		errs.Add("name", "is required")
	}
	if d.Type == "" {
		d.Type = DefaultType
	}
	for _, f := range []**string{&d.Category, &d.Industry, &d.Phone, &d.City, &d.RegistrationNumber} {
		*f = squeezedOrNil(*f)
	}
	for _, f := range []**string{&d.Address, &d.About} {
		*f = trimmedOrNil(*f) // keep line breaks
	}
	if d.Country != nil {
		c := strings.ToUpper(strings.TrimSpace(*d.Country))
		d.Country = &c
	}
	if w := trimmedOrNil(d.Website); w != nil {
		normal, ok := NormalizeWebsite(*w)
		if !ok {
			errs.Add("website", "is not a web address, e.g. example.com")
		}
		d.Website = &normal
	} else {
		d.Website = nil
	}
	return d, errs
}

// NormalizeWebsite turns "Example.com/careers" into "https://example.com/careers".
func NormalizeWebsite(raw string) (string, bool) {
	s := raw
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.Contains(u.Hostname(), ".") ||
		strings.ContainsAny(u.Hostname(), " _") {
		return raw, false
	}
	u.Host = strings.ToLower(u.Host)
	out := u.String()
	if u.Path == "/" && u.RawQuery == "" && u.Fragment == "" {
		out = strings.TrimSuffix(out, "/")
	}
	return out, true
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
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
