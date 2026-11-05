package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// FieldRules is what this module needs from the settings module: the
// fields a company made mandatory, and its list of sources.
type FieldRules interface {
	Mandatory(ctx context.Context, tx pgx.Tx, form string) ([]string, error)
	HasOption(ctx context.Context, tx pgx.Tx, list, key string) (bool, error)
}

// filled reports whether a profile has a value for a field (its API name).
func filled(p Profile, field string) bool {
	set := func(s *string) bool { return s != nil && *s != "" }
	switch field {
	case "email":
		return set(p.Email)
	case "phone":
		return set(p.Phone)
	case "nationality":
		return set(p.Nationality)
	case "residenceCountry":
		return set(p.ResidenceCountry)
	case "linkedinUrl":
		return set(p.LinkedinURL)
	case "location":
		return set(p.Location)
	case "currentTitle":
		return set(p.CurrentTitle)
	case "currentCompany":
		return set(p.CurrentCompany)
	case "industry":
		return set(p.Industry)
	case "source":
		return set(p.Source)
	case "noticePeriod":
		return p.NoticePeriod != nil
	case "workAuthorization":
		return p.WorkAuthorization != nil
	}
	return true
}

// MissingMandatory checks a company's mandatory fields: a new candidate
// (before nil) needs them all; a change may not empty one, so records from
// before a field became mandatory can still be edited.
func MissingMandatory(mandatory []string, before *Profile, after Profile) apperr.FieldErrors {
	var errs apperr.FieldErrors
	for _, f := range mandatory {
		if !filled(after, f) && (before == nil || filled(*before, f)) {
			errs.Add(f, "is mandatory in your company's settings")
		}
	}
	return errs
}

// checkFields applies the company's settings to a person's change: its
// mandatory fields, and a source from its list.
func (s *Service) checkFields(ctx context.Context, tx pgx.Tx, before *Profile, after Profile) error {
	if s.rules == nil {
		return nil
	}
	mandatory, err := s.rules.Mandatory(ctx, tx, "candidate")
	if err != nil {
		return err
	}
	errs := MissingMandatory(mandatory, before, after)
	if src := after.Source; src != nil && (before == nil || before.Source == nil || *before.Source != *src) {
		ok, err := s.rules.HasOption(ctx, tx, "candidate_source", *src)
		if err != nil {
			return err
		}
		if !ok {
			errs.Add("source", "is not one of your company's sources (Settings)")
		}
	}
	return errs.Err()
}
