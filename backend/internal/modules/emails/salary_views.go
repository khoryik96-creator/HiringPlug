package emails

// Who was shown a salary (salaryview): an email being written whose fields
// came back filled with a candidate's salary (a preview's field values, a
// draft from a template), and an email sent with a salary in it, are
// counted for the candidates whose salary it was; so is a sent email with a
// salary read again (a candidate's or contact's emails, a copy of a
// candidate's data). Automatic emails (the system's) are never counted.

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
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

// salaryFilled: Fill or FillHTML put a salary into one of the texts, that
// is, a {{placeholder}} of a salary field that values fill. A draft leaves
// [Field] tokens as they are (the composer fills them, and counts that).
func salaryFilled(values map[string]string, texts ...string) bool {
	for _, t := range texts {
		for _, m := range placeholderRe.FindAllStringSubmatch(t, -1) {
			if f, ok := fieldByKey(m[1]); ok && f.Salary {
				if _, filled := values[m[1]]; filled {
					return true
				}
			}
		}
	}
	return false
}

// whoseFields is the candidate whose details fill a message's own fields:
// the recipient, when the email goes to a candidate (shortlistValues leaves
// out the first shortlisted candidate's details, recipientValues puts in the
// recipient's); otherwise the candidate it is about.
func whoseFields(about, recipient *string) *string {
	if recipient != nil {
		return recipient
	}
	return about
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

// salaryReaders are the candidates whose salary sent emails with one may
// show, by email: the shortlisted candidates of an email about several (a
// salary in it may be any of theirs); only the recipient, for one to a
// candidate about others (it never shows another candidate's salary,
// salaryOnlyTheirOwn); otherwise the candidate it is about. The stored
// message no longer marks whose each salary is.
func salaryReaders(emails []store.Email, shortlisted []store.EmailCandidateIDsRow) []string {
	listed := map[string][]string{}
	for _, r := range shortlisted {
		listed[r.EmailID] = append(listed[r.EmailID], r.CandidateID)
	}
	var out []string
	for _, e := range emails {
		if !e.Salary {
			continue
		}
		list := listed[e.ID]
		toCandidate := e.CandidateID != nil && len(list) > 0 && !slices.Contains(list, *e.CandidateID)
		switch {
		case toCandidate || len(list) == 0:
			if e.CandidateID != nil {
				out = append(out, *e.CandidateID)
			}
		default:
			out = append(out, list...)
		}
	}
	return out
}

// salaryEmailsRead counts sent emails with a salary that are read again,
// once for the list, for each candidate whose salary they may show.
func (s *Service) salaryEmailsRead(ctx context.Context, tx pgx.Tx, emails []store.Email) error {
	var ids []string
	for _, e := range emails {
		if e.Salary {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 || s.salaryViews == nil {
		return nil
	}
	shortlisted, err := store.New(tx).EmailCandidateIDs(ctx, ids)
	if err != nil {
		return err
	}
	return salaryview.In(ctx, tx, s.salaryViews, salaryview.EmailRead, salaryReaders(emails, shortlisted)...)
}
