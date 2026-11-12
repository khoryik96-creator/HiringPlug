package emails

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func invalidRecipient() error {
	return apperr.Validation(apperr.FieldErrors{{Field: "recipientCandidateId", Message: "choose either a contact or a candidate recipient, alongside selected candidate details"}})
}

// Recipients and selected candidate packages are independent. Greeting fields belong to the recipient.
func (s *Service) recipientValues(ctx context.Context, tx pgx.Tx, p authz.Principal, values map[string]string, contactID, recipientID *string, loc *time.Location) error {
	if contactID == nil && recipientID == nil {
		return nil
	}
	recipient, err := s.values(ctx, tx, p, recipientID, nil, nil, contactID, loc)
	if err != nil {
		return err
	}
	for key, value := range recipient {
		values[key] = value
	}
	return nil
}

// salaryOnlyTheirOwn: an email to a candidate that shows salary may describe
// only that candidate; another candidate's salary never goes to a candidate.
func salaryOnlyTheirOwn(recipientID *string, list []Shortlisted, showsSalary bool) error {
	if recipientID == nil || !showsSalary {
		return nil
	}
	for _, c := range list {
		if c.CandidateID != *recipientID {
			return apperr.Validation(apperr.FieldErrors{{Field: "bodyHtml", Message: "another candidate's salary cannot be sent to a candidate; untick the salary details"}})
		}
	}
	return nil
}
