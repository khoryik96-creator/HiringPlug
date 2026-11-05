package jobs

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// FieldRules is what this module needs from the settings module: the
// fields a company made mandatory, and its words for work types.
type FieldRules interface {
	Mandatory(ctx context.Context, tx pgx.Tx, form string) ([]string, error)
	Label(ctx context.Context, tx pgx.Tx, list, key string) (string, error)
}

func filled(d Details, field string) bool {
	set := func(s *string) bool { return s != nil && *s != "" }
	switch field {
	case "location":
		return set(d.Location)
	case "workType":
		return set(d.WorkType)
	case "workArrangement":
		return set(d.WorkArrangement)
	case "contactId":
		return set(d.ContactID)
	case "description":
		return set(d.Description)
	}
	return true
}

// MissingMandatory checks a company's mandatory fields: a new job (before
// nil) needs them all; a change may not empty one.
func MissingMandatory(mandatory []string, before *Details, after Details) apperr.FieldErrors {
	var errs apperr.FieldErrors
	for _, f := range mandatory {
		if !filled(after, f) && (before == nil || filled(*before, f)) {
			errs.Add(f, "is mandatory in your company's settings")
		}
	}
	return errs
}

func (s *Service) checkMandatory(ctx context.Context, tx pgx.Tx, before *Details, after Details) error {
	if s.rules == nil {
		return nil
	}
	mandatory, err := s.rules.Mandatory(ctx, tx, "job")
	if err != nil {
		return err
	}
	return MissingMandatory(mandatory, before, after).Err()
}
