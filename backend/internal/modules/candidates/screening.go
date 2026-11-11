package candidates

// Screening calls: what a recruiter asks on the first call updates the
// candidate in one step, and their notes become a screening note.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
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
	SalaryNotes     Change[*string]
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
	CalledOn  *time.Time // nil: today
	CalledBy  *string    // nil: whoever records it
}

// ScreeningResult is the candidate after the call.
type ScreeningResult struct {
	View          View
	NoteID        *string
	SourceKept    bool
	SalaryApplied bool // false when a later call already set the salary
	Call          ScreeningCall
}

// ScreeningCall is one screening call kept on a candidate.
type ScreeningCall struct {
	ID, CalledBy   string
	CalledByName   *string
	CalledOn       time.Time
	NoteID         *string
	SalaryRecorded bool
	SalarySnapshot *api.ScreeningSalarySnapshot
	CreatedAt      time.Time
}

// SalaryFollows reports whether a call on calledOn sets the salary, given the
// date of the latest call that set it (nil: none): a call on the same day or
// later does, an earlier one keeps the later call's salary.
func SalaryFollows(calledOn time.Time, latest *time.Time) bool {
	return latest == nil || !calledOn.Before(*latest)
}

// today is the date now, in UTC (a call's date is a calendar day).
func today() time.Time {
	y, m, d := time.Now().UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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
	calledOn, calledBy := today(), p.UserID
	if in.CalledOn != nil {
		calledOn = *in.CalledOn
	}
	if in.CalledBy != nil {
		calledBy = *in.CalledBy
	}
	if calledOn.After(today().AddDate(0, 0, 1)) { // a day's grace for time zones ahead of UTC
		var errs apperr.FieldErrors
		errs.Add("calledOn", "cannot be in the future")
		return ScreeningResult{}, errs.Err()
	}
	var out ScreeningResult
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		names, err := s.users.UserNames(ctx, tx, []string{calledBy})
		if err != nil {
			return err
		}
		byName, known := names[calledBy]
		if !known {
			var errs apperr.FieldErrors
			errs.Add("calledBy", "is not one of your users")
			return errs.Err()
		}
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
		latest, err := q.LatestSalaryCall(ctx, id)
		if err != nil {
			return err
		}
		var latestOn *time.Time
		if latest.Valid {
			latestOn = &latest.Time
		}
		out.SalaryApplied = in.Salary != nil && SalaryFollows(calledOn, latestOn)
		if out.SalaryApplied {
			comp, err := setCompensationIn(ctx, tx, id, in.Salary.ExpectedVersion, in.Salary.Current, in.Salary.Expected, in.Salary.SalaryNotes)
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
			body := notes
			if calledBy != p.UserID || !calledOn.Equal(today()) {
				// Who made the call and when, when that is not the note's own author and day.
				body = "Call on " + calledOn.Format("2 Jan 2006") + " by " + byName + "\n\n" + notes
			}
			noteID, err := s.notes.AddNote(ctx, tx, p, "candidate", id, "screening", body, in.Sensitive)
			if err != nil {
				return err
			}
			out.NoteID = &noteID
		}
		out.View = view
		row, err := q.InsertScreeningCall(ctx, store.InsertScreeningCallParams{
			CandidateID: id, CalledOn: pgtype.Date{Time: calledOn, Valid: true}, CalledBy: calledBy, NoteID: out.NoteID, SalaryRecorded: out.SalaryApplied,
		})
		if err != nil {
			return err
		}
		out.Call = callFromRow(row, &byName)
		if err := saveCallSalary(ctx, tx, p, &out.Call, view.Compensation, in.Salary); err != nil {
			return err
		}
		// What was recorded, never the values (salary stays out of the log).
		return audit.Record(ctx, tx, audit.Entry{
			Action: "candidate.screened", EntityType: "candidate", EntityID: id,
			Changes: map[string]any{"salary": out.SalaryApplied, "salarySentButOlder": in.Salary != nil && !out.SalaryApplied,
				"note": out.NoteID != nil, "calledOn": calledOn.Format(time.DateOnly), "calledBy": calledBy},
		})
	})
	return out, err
}

// ScreeningCalls lists a candidate's calls, latest first.
func (s *Service) ScreeningCalls(ctx context.Context, p authz.Principal, id string) ([]ScreeningCall, error) {
	var out []ScreeningCall
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetCandidate(ctx, id); database.IsNoRows(err) { // as their role may see candidates
			return apperr.NotFound("candidate")
		} else if err != nil {
			return err
		}
		rows, err := q.ListScreeningCalls(ctx, id)
		if err != nil {
			return err
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.CalledBy
		}
		names, err := s.users.UserNames(ctx, tx, ids)
		if err != nil {
			return err
		}
		out = make([]ScreeningCall, len(rows))
		for i, r := range rows {
			var name *string
			if n, ok := names[r.CalledBy]; ok {
				name = &n
			}
			out[i] = callFromRow(r, name)
		}
		return loadCallSalaries(ctx, tx, out)
	})
	return out, err
}

func callFromRow(r store.ScreeningCall, name *string) ScreeningCall {
	return ScreeningCall{ID: r.ID, CalledBy: r.CalledBy, CalledByName: name, CalledOn: r.CalledOn.Time, NoteID: r.NoteID,
		SalaryRecorded: r.SalaryRecorded, CreatedAt: r.CreatedAt}
}

// ListScreeningCalls handles GET /v1/candidates/{candidateId}/screening-calls.
func (h *HTTP) ListScreeningCalls(ctx context.Context, req api.ListScreeningCallsRequestObject) (api.ListScreeningCallsResponseObject, error) {
	calls, err := h.svc.ScreeningCalls(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.ListScreeningCalls200JSONResponse{Items: make([]api.ScreeningCallSummary, len(calls))}
	for i, c := range calls {
		out.Items[i] = callToAPI(c)
	}
	return out, nil
}

func callToAPI(c ScreeningCall) api.ScreeningCallSummary {
	return api.ScreeningCallSummary{Id: c.ID, CalledOn: openapi_types.Date{Time: c.CalledOn}, CalledBy: c.CalledBy, CalledByName: orNull(c.CalledByName),
		NoteId: orNull(c.NoteID), SalaryRecorded: c.SalaryRecorded, CreatedAt: c.CreatedAt, SalarySnapshot: c.SalarySnapshot}
}

// RecordScreeningCall handles POST /v1/candidates/{candidateId}/screening-calls.
func (h *HTTP) RecordScreeningCall(ctx context.Context, req api.RecordScreeningCallRequestObject) (api.RecordScreeningCallResponseObject, error) {
	b := req.Body
	in := Screening{
		Version: b.Version, Notes: deref(b.Notes), Sensitive: deref(b.Sensitive), Source: b.Source, CalledBy: b.CalledBy,
		Patch: ProfilePatch{
			IndustrySelections: choicesFromAPI(b.IndustrySelections),
			Nationality:        change(b.Nationality, identity[string]),
			WorkAuthorization:  Change[*WorkAuthorization]{Set: b.WorkAuthorization.IsSpecified(), Value: workAuthorizationFromAPI(b.WorkAuthorization)},
			NoticePeriod:       Change[*NoticePeriod]{Set: b.NoticePeriod.IsSpecified(), Value: noticePeriodFromAPI(b.NoticePeriod)},
		},
	}
	if c := b.Compensation; c != nil {
		in.Salary = &SalaryInput{ExpectedVersion: c.ExpectedVersion, SalaryNotes: change(c.SalaryNotes, identity[string])}
		in.Salary.Current, in.Salary.Expected = salaryFromAPI(*c)
	}
	if b.CalledOn != nil {
		d := b.CalledOn.Time
		in.CalledOn = &d
	}
	res, err := h.svc.RecordScreening(ctx, authz.MustFromContext(ctx), req.CandidateId, in)
	if err != nil {
		return nil, err
	}
	out := api.RecordScreeningCall200JSONResponse{Candidate: candidateToAPI(res.View), SourceKept: res.SourceKept, NoteId: orNull(res.NoteID),
		SalaryApplied: res.SalaryApplied, Call: callToAPI(res.Call)}
	return out, nil
}
