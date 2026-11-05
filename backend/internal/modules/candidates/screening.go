package candidates

// Screening calls: what a recruiter asks on the first call updates the
// candidate in one step, and their notes become a screening note.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Notes is what screening calls need from the notes module.
type Notes interface {
	AddNote(ctx context.Context, tx pgx.Tx, p authz.Principal, subjectType, subjectID, kind, body string, sensitive bool) (string, error)
}

// WriteNotesWith sets the notes module (called by internal/app: notes
// depends on this module).
func (m *Module) WriteNotesWith(n Notes) { m.service.notes = n }

// SalaryInput is salary details to save, as PUT .../compensation takes them.
type SalaryInput struct {
	ExpectedVersion int
	Current         *SalaryAmount
	Expected        *SalaryRange
}

// Screening is what a screening call records. Patch holds only nationality,
// work authorization and notice period; Source is set only when the
// candidate has none.
type Screening struct {
	Version   int
	Notes     string
	Sensitive bool
	Patch     ProfilePatch
	Source    *string
	Salary    *SalaryInput
}

// ScreeningResult is the candidate after the call.
type ScreeningResult struct {
	View       View
	NoteID     *string
	SourceKept bool
}

// RecordScreening saves a screening call in one transaction: the profile
// fields asked, the salary, and the notes.
func (s *Service) RecordScreening(ctx context.Context, p authz.Principal, id string, in Screening) (ScreeningResult, error) {
	notes := strings.TrimSpace(in.Notes)
	switch {
	case in.Salary != nil && !p.Can(api.PermissionCandidatesCompensationUpdate):
		return ScreeningResult{}, apperr.New(api.ErrorCodeFORBIDDEN, "Your role may not change salary details.")
	case notes != "" && !p.Can(api.PermissionNotesCreate):
		return ScreeningResult{}, apperr.New(api.ErrorCodeFORBIDDEN, "Your role may not write notes.")
	}
	if in.Salary != nil {
		if err := ValidateCompensation(in.Salary.Current, in.Salary.Expected).Err(); err != nil {
			return ScreeningResult{}, err
		}
	}
	var out ScreeningResult
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		change := func(before Profile) Profile {
			after := in.Patch.Apply(before)
			out.SourceKept = in.Source != nil && before.Source != nil
			if in.Source != nil && before.Source == nil {
				after.Source = in.Source
			}
			return after
		}
		// A call that changes nothing in the profile (only notes or salary)
		// leaves it alone, even if someone else changed it meanwhile.
		view, err := s.updateIn(ctx, tx, p, id, in.Version, change, true)
		if err != nil {
			return err
		}
		if in.Salary != nil {
			comp, err := setCompensationIn(ctx, tx, id, in.Salary.ExpectedVersion, in.Salary.Current, in.Salary.Expected)
			if err != nil {
				return err
			}
			if !view.CompensationRestricted {
				view.Compensation = &comp
			}
		}
		if notes != "" {
			if s.notes == nil {
				return apperr.New(api.ErrorCodeINTERNAL, "notes are not available")
			}
			noteID, err := s.notes.AddNote(ctx, tx, p, "candidate", id, "screening", notes, in.Sensitive)
			if err != nil {
				return err
			}
			out.NoteID = &noteID
		}
		out.View = view
		// What was recorded, never the values (salary stays out of the log).
		return audit.Record(ctx, tx, audit.Entry{
			Action: "candidate.screened", EntityType: "candidate", EntityID: id,
			Changes: map[string]any{"salary": in.Salary != nil, "note": out.NoteID != nil},
		})
	})
	return out, err
}

// RecordScreeningCall handles POST /v1/candidates/{candidateId}/screening-calls.
func (h *HTTP) RecordScreeningCall(ctx context.Context, req api.RecordScreeningCallRequestObject) (api.RecordScreeningCallResponseObject, error) {
	b := req.Body
	in := Screening{
		Version: b.Version, Notes: deref(b.Notes), Sensitive: deref(b.Sensitive), Source: b.Source,
		Patch: ProfilePatch{
			Nationality:       change(b.Nationality, identity[string]),
			WorkAuthorization: Change[*WorkAuthorization]{Set: b.WorkAuthorization.IsSpecified(), Value: workAuthorizationFromAPI(b.WorkAuthorization)},
			NoticePeriod:      Change[*NoticePeriod]{Set: b.NoticePeriod.IsSpecified(), Value: noticePeriodFromAPI(b.NoticePeriod)},
		},
	}
	if c := b.Compensation; c != nil {
		in.Salary = &SalaryInput{ExpectedVersion: c.ExpectedVersion}
		in.Salary.Current, in.Salary.Expected = salaryFromAPI(*c)
	}
	res, err := h.svc.RecordScreening(ctx, authz.MustFromContext(ctx), req.CandidateId, in)
	if err != nil {
		return nil, err
	}
	out := api.RecordScreeningCall200JSONResponse{Candidate: candidateToAPI(res.View), SourceKept: res.SourceKept, NoteId: orNull(res.NoteID)}
	return out, nil
}
