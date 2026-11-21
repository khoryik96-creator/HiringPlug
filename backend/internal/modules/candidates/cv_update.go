package candidates

// A candidate's profile kept up to date from their newest CV: when a CV is
// uploaded to someone already here and has been read, what it says about
// where they are now (job title, employer, industry, location) replaces what
// the profile had, empty fields are filled, new skills are added, and the
// work history and education are taken from it when it has at least as much.
// A CV older than the profile's history (an old CV found later) only fills
// empty fields.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// UpdateFrom is the profile and history after a newer CV: before with the
// CV's current job, industry and location in place of its own (when the CV
// names them and is not older than the history), empty fields filled, skills
// joined, and the CV's history when it is at least as long. The email and the
// name are left to the caller (the email may be someone else's).
func UpdateFrom(before Profile, history History, cv FromCV) (Profile, History) {
	fromCV := cv.Profile
	fromCV.Email = nil
	after, _ := FillFrom(before, fromCV)
	if !olderThan(cv.History.Work, history.Work) {
		replace := func(field **string, v *string) {
			if v != nil && strings.TrimSpace(*v) != "" && !sameText(*field, v) {
				*field = v
			}
		}
		replace(&after.CurrentTitle, cv.Profile.CurrentTitle)
		replace(&after.CurrentCompany, cv.Profile.CurrentCompany)
		replace(&after.Industry, cv.Profile.Industry)
		replace(&after.Location, cv.Profile.Location)
		if len(cv.History.Work) > 0 && len(cv.History.Work) >= len(history.Work) {
			history.Work = withDescriptions(cv.History.Work, history.Work)
		}
		if len(cv.History.Education) > 0 && len(cv.History.Education) >= len(history.Education) {
			history.Education = cv.History.Education
		}
	} else {
		if len(history.Work) == 0 {
			history.Work = cv.History.Work
		}
		if len(history.Education) == 0 {
			history.Education = cv.History.Education
		}
	}
	return after, history
}

// withDescriptions is the CV's jobs, each keeping the description written on
// the profile's same job (same employer, and the same title or start), which
// a CV does not give.
func withDescriptions(cv, profile []Job) []Job {
	out := make([]Job, len(cv))
	copy(out, cv)
	used := make([]bool, len(profile))
	for i := range out {
		if out[i].Description != nil && strings.TrimSpace(*out[i].Description) != "" {
			continue
		}
		for k, p := range profile {
			if used[k] || p.Description == nil || !sameText(p.Company, out[i].Company) {
				continue
			}
			if sameText(p.Title, out[i].Title) || (p.StartDate != nil && sameText(p.StartDate, out[i].StartDate)) {
				out[i].Description, used[k] = p.Description, true
				break
			}
		}
	}
	return out
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return strings.EqualFold(strings.TrimSpace(*a), strings.TrimSpace(*b))
}

// olderThan: the CV's latest job started before the profile's latest one (an
// old CV): "YYYY" and "YYYY-MM" compare as text. Undated histories are not older.
func olderThan(cv, profile []Job) bool {
	a, b := latestStart(cv), latestStart(profile)
	return a != "" && b != "" && a < b[:min(len(a), len(b))]
}

func latestStart(work []Job) string {
	latest := ""
	for _, j := range work {
		if j.StartDate != nil && *j.StartDate > latest {
			latest = *j.StartDate
		}
	}
	return latest
}

// UpdateFromCV keeps a candidate up to date from the CV just read, when it is
// their newest (UpdateFrom). The CV's email is used only when the profile has
// none and it is nobody else's. A CV value the rules refuse is left out. An
// audit entry names what changed and from which CV; nothing changed, nothing
// written.
func (m *Module) UpdateFromCV(ctx context.Context, tx pgx.Tx, id, documentID string, cv FromCV) error {
	if m.service.industries != nil {
		if err := m.service.industries.LockIn(ctx, tx); err != nil {
			return err
		}
	}
	q := store.New(tx)
	row, err := q.GetCandidate(ctx, id)
	if err != nil {
		return err
	}
	before := fromRow(row).Profile
	history, err := loadHistory(ctx, q, id)
	if err != nil {
		return err
	}
	after, newHistory := UpdateFrom(before, history, cv)
	if e := cv.Profile.Email; e != nil && after.Email == nil {
		lower := strings.ToLower(strings.TrimSpace(*e))
		existing, err := q.FindCandidateIDByEmail(ctx, &lower)
		switch {
		case database.IsNoRows(err) || (err == nil && existing == id):
			after.Email = e
		case err != nil:
			return err
		}
	}
	normal, errs := NormalizeProfile(after, m.service.phoneRegion)
	for len(errs) > 0 { // leave out what the rules refuse, keep the rest
		dropped := false
		for _, e := range errs {
			dropped = dropField(&after, before, e.Field) || dropped
		}
		if !dropped {
			return errs.Err()
		}
		normal, errs = NormalizeProfile(after, m.service.phoneRegion)
	}
	changes := audit.Diff(profileFields(before), profileFields(normal))
	if len(changes) > 0 {
		if _, err := q.UpdateCandidate(ctx, updateParams(id, int(row.Version), normal)); err != nil {
			return err
		}
		m.service.used.learnAny(normal)
		if err := m.service.saveIndustries(ctx, tx, id, normal); err != nil {
			return err
		}
	}
	if clean, changed := historyChange(newHistory, history); changed {
		if err := saveHistory(ctx, q, id, clean); err != nil {
			return err
		}
		if len(changes) == 0 { // the profile is as it was: the history's change still moves its version on (open editors check it)
			if _, err := q.TouchCandidate(ctx, store.TouchCandidateParams{ID: id, ExpectedVersion: row.Version}); err != nil {
				return err
			}
		}
		changes["workHistory"] = map[string]any{"from": len(history.Work), "to": len(clean.Work)}
	}
	if len(changes) == 0 {
		return nil
	}
	changes["documentId"] = documentID
	return audit.Record(ctx, tx, audit.Entry{
		Action: "candidate.updated_from_cv", EntityType: "candidate", EntityID: id, Changes: changes,
	})
}

// historyChange is the new history tidied as it is stored, and whether it
// differs from the stored one: a CV saying the same again changes nothing. A
// history the rules refuse is no change.
func historyChange(next, stored History) (History, bool) {
	clean, errs := NormalizeHistory(next)
	if len(errs) > 0 {
		return stored, false
	}
	return clean, !sameJobs(clean.Work, stored.Work) || !sameSlice(clean.Education, stored.Education)
}

// sameJobs: the same jobs, by title, company and dates.
func sameJobs(a, b []Job) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameText(a[i].Title, b[i].Title) || !sameText(a[i].Company, b[i].Company) ||
			!sameText(a[i].StartDate, b[i].StartDate) || !sameText(a[i].EndDate, b[i].EndDate) || a[i].Current != b[i].Current {
			return false
		}
	}
	return true
}

func sameSlice(a, b []Qualification) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameText(a[i].Qualification, b[i].Qualification) || !sameText(a[i].Institution, b[i].Institution) ||
			(a[i].EndYear == nil) != (b[i].EndYear == nil) || (a[i].EndYear != nil && *a[i].EndYear != *b[i].EndYear) {
			return false
		}
	}
	return true
}
