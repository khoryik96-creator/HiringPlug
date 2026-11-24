package emails

// Who was shown a salary (salaryview): an email being written whose fields
// came back filled with a candidate's salary (a preview's field values, a
// draft from a template), and an email sent with a salary in it, are
// counted for the candidates whose salary it was. Automatic emails (the
// system's) are never counted.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// RecordSalaryViewsWith counts where this module shows a salary (the
// auditlog module).
func (m *Module) RecordSalaryViewsWith(r salaryview.Recorder) { m.service.salaryViews = r }

// fieldValues are the values by field token, for the composer.
func fieldValues(values map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range Fields {
		if v, ok := values[f.Key]; ok && v != "" {
			out[f.Token()] = v
		}
	}
	return out
}

// tokensShowSalary: a salary field has a value among the composer's field
// values (by token).
func tokensShowSalary(byToken map[string]string) bool {
	for _, f := range Fields {
		if f.Salary && byToken[f.Token()] != "" {
			return true
		}
	}
	return false
}

// salaryFilled: one of the texts has a salary field that values fill.
func salaryFilled(values map[string]string, texts ...string) bool {
	for _, t := range texts {
		if t != "" && FillFields(t, values, false).Salary {
			return true
		}
	}
	return false
}

// salaryFieldsShown counts the composer's field values when they hold the
// candidate's salary.
func (s *Service) salaryFieldsShown(ctx context.Context, tx pgx.Tx, byToken map[string]string, candidateID *string) error {
	if candidateID == nil || !tokensShowSalary(byToken) {
		return nil
	}
	return salaryview.In(ctx, tx, s.salaryViews, salaryview.EmailFields, *candidateID)
}

// salarySent counts an email sent with a salary: for each salary field, the
// candidate whose details row holds it, or valuesOf (whose details fill
// the message's own fields) when it is in the message itself, or when no
// field in the message says whose (the subject, a plain message).
// A formatted CV attached with the salary in it is counted as a CV shown.
func (s *Service) salarySent(ctx context.Context, tx pgx.Tx, salary bool, valuesOf *string, bodyHTML string, cvs ...CVFile) error {
	var cvsWithSalary []string
	for _, cv := range cvs {
		if cv.Salary {
			cvsWithSalary = append(cvsWithSalary, cv.CandidateID)
		}
	}
	if err := salaryview.In(ctx, tx, s.salaryViews, salaryview.CV, cvsWithSalary...); err != nil {
		return err
	}
	if !salary {
		return nil
	}
	var whose []string
	owners := salaryOwners(bodyHTML)
	own := len(owners) == 0 // in the subject or a plain message
	for _, owner := range owners {
		if strings.HasPrefix(owner, "cand_") {
			whose = append(whose, owner)
		} else {
			own = true
		}
	}
	if own && valuesOf != nil {
		whose = append(whose, *valuesOf)
	}
	return salaryview.In(ctx, tx, s.salaryViews, salaryview.EmailSent, whose...)
}
