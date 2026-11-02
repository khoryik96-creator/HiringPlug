package interviews

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the interview use cases.
type Service struct {
	pool         *pgxpool.Pool
	applications Applications
	users        Users
}

// Person is a user with their name.
type Person struct{ ID, Name string }

// View is an interview as a user may see it.
type View struct {
	ID             string
	Version        int
	App            Application
	Details        Details
	Interviewers   []Person
	Feedback       []FeedbackEntry
	Names          map[string]string // user id -> name, for feedback
	FeedbackHidden int
	MyFeedbackDue  bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Schedule adds an interview to an application, and moves the application to
// the interview stage if it is at an earlier one.
func (s *Service) Schedule(ctx context.Context, p authz.Principal, appID string, d Details, ignoreClashes bool) (View, error) {
	d.Status = "scheduled"
	d, errs := Normalize(d)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.applications.Briefs(ctx, tx, []string{appID})
		if err != nil {
			return err
		}
		app, ok := apps[appID]
		if !ok {
			return apperr.NotFound("application")
		}
		if problem := CanSchedule(app.Stage); problem != "" {
			return apperr.New(api.ErrorCodeINTERVIEWNOTALLOWED, problem)
		}
		q := store.New(tx)
		if err := s.check(ctx, tx, q, "", d, ignoreClashes); err != nil {
			return err
		}
		row, err := q.InsertInterview(ctx, store.InsertInterviewParams{
			ApplicationID: appID, Title: d.Title, StartsAt: d.StartsAt, EndsAt: d.Ends(), DurationMinutes: int32(d.Duration), //nolint:gosec // 5..480 per the contract
			Mode: d.Mode, Location: d.Location, Instructions: d.Instructions, InterviewerIds: d.Interviewers,
		})
		if err != nil {
			return err
		}
		if err := s.applications.ReachInterview(ctx, tx, appID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "interview.scheduled", EntityType: "interview", EntityID: row.ID,
			Changes: map[string]any{"applicationId": appID, "title": d.Title, "startsAt": d.StartsAt, "interviewers": d.Interviewers},
		}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// check refuses unknown or inactive interviewers, and (unless asked not to)
// interviewers who are busy at that time.
func (s *Service) check(ctx context.Context, tx pgx.Tx, q *store.Queries, selfID string, d Details, ignoreClashes bool) error {
	active, err := s.users.ActiveUsers(ctx, tx, d.Interviewers)
	if err != nil {
		return err
	}
	for _, id := range d.Interviewers {
		perms, ok := active[id]
		if !ok {
			return apperr.Validation(apperr.FieldErrors{{Field: "interviewerIds", Message: "includes someone who is not an active user"}})
		}
		if !slices.Contains(perms, "interviews:feedback") {
			return apperr.Validation(apperr.FieldErrors{{Field: "interviewerIds", Message: "includes someone whose role cannot give interview feedback"}})
		}
	}
	if ignoreClashes || d.Status != "scheduled" {
		return nil
	}
	clashes, err := q.Clashes(ctx, store.ClashesParams{ExcludeID: selfID, InterviewerIds: d.Interviewers, StartsAt: d.StartsAt, EndsAt: d.Ends()})
	if err != nil || len(clashes) == 0 {
		return err
	}
	names, err := s.users.UserNames(ctx, tx, d.Interviewers)
	if err != nil {
		return err
	}
	list := make([]map[string]any, len(clashes))
	for i, c := range clashes {
		busy := []string{}
		for _, id := range c.InterviewerIds {
			if slices.Contains(d.Interviewers, id) {
				busy = append(busy, names[id])
			}
		}
		list[i] = map[string]any{"interviewId": c.ID, "title": c.Title, "startsAt": c.StartsAt, "interviewers": busy}
	}
	return apperr.New(api.ErrorCodeINTERVIEWCLASH, "An interviewer already has an interview at that time.").WithDetail("clashes", list)
}

// ForApplication returns an application's interviews, soonest first.
func (s *Service) ForApplication(ctx context.Context, p authz.Principal, appID string) ([]View, error) {
	var out []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.applications.Briefs(ctx, tx, []string{appID})
		if err != nil {
			return err
		}
		if _, ok := apps[appID]; !ok {
			return apperr.NotFound("application")
		}
		rows, err := store.New(tx).ListApplicationInterviews(ctx, appID)
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, p, rows)
		return err
	})
	return out, err
}

// List returns interviews in a time range; mine keeps the user's own, and
// feedbackDue the past ones they still owe feedback on.
func (s *Service) List(ctx context.Context, p authz.Principal, from, to *time.Time, mine, feedbackDue bool) ([]View, error) {
	var out []View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var rows []store.Interview
		var err error
		if feedbackDue {
			rows, err = q.FeedbackDue(ctx, p.UserID)
		} else {
			params := store.ListInterviewsParams{FromTime: from, ToTime: to}
			if mine {
				params.Interviewer = &p.UserID
			}
			rows, err = q.ListInterviews(ctx, params)
		}
		if err != nil {
			return err
		}
		out, err = s.views(ctx, tx, p, rows)
		return err
	})
	return out, err
}

// Get returns one interview.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (View, error) {
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getRow(ctx, store.New(tx), id)
		if err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// Update reschedules, changes, cancels or completes an interview.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch Patch, ignoreClashes bool) (View, error) {
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, id)
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("interview").WithDetail("currentVersion", row.Version)
		}
		before := detailsOf(row)
		after, errs := Normalize(patch.Apply(before))
		if err := errs.Err(); err != nil {
			return err
		}
		changes := audit.Diff(fields(before), fields(after))
		if len(changes) == 0 {
			out, err = s.one(ctx, tx, p, row)
			return err
		}
		if err := s.check(ctx, tx, q, id, after, ignoreClashes); err != nil {
			return err
		}
		updated, err := q.UpdateInterview(ctx, store.UpdateInterviewParams{
			ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small
			Title: after.Title, StartsAt: after.StartsAt, EndsAt: after.Ends(), DurationMinutes: int32(after.Duration), //nolint:gosec // 5..480
			Mode: after.Mode, Location: after.Location, Instructions: after.Instructions,
			InterviewerIds: after.Interviewers, Status: after.Status,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("interview")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "interview.updated", EntityType: "interview", EntityID: id, Changes: changes}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, updated)
		return err
	})
	return out, err
}

// PutFeedback records (or replaces) the user's own feedback.
func (s *Service) PutFeedback(ctx context.Context, p authz.Principal, id, recommendation string, comments *string) (View, error) {
	comments = trimmedOrNil(comments)
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, id)
		if err != nil {
			return err
		}
		if problem := FeedbackProblem(p.UserID, detailsOf(row)); problem != "" {
			return apperr.New(api.ErrorCodeINTERVIEWNOTALLOWED, problem)
		}
		if err := q.UpsertFeedback(ctx, store.UpsertFeedbackParams{
			InterviewID: id, UserID: p.UserID, Recommendation: recommendation, Comments: comments,
		}); err != nil {
			return err
		}
		// The recommendation only: comments stay on the interview.
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "interview.feedback_given", EntityType: "interview", EntityID: id,
			Changes: map[string]any{"recommendation": recommendation},
		}); err != nil {
			return err
		}
		out, err = s.one(ctx, tx, p, row)
		return err
	})
	return out, err
}

// CalendarFile returns the interview as an .ics file.
func (s *Service) CalendarFile(ctx context.Context, p authz.Principal, id string) (string, error) {
	v, err := s.Get(ctx, p, id)
	if err != nil {
		return "", err
	}
	return Calendar(v.ID, v.Version, v.Details, v.App.CandidateName, v.App.JobTitle, time.Now()), nil
}

func (s *Service) one(ctx context.Context, tx pgx.Tx, p authz.Principal, row store.Interview) (View, error) {
	views, err := s.views(ctx, tx, p, []store.Interview{row})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

// views adds what the page shows: the application, people's names, and the
// feedback this user may see.
func (s *Service) views(ctx context.Context, tx pgx.Tx, p authz.Principal, rows []store.Interview) ([]View, error) {
	appIDs, ids, userIDs := []string{}, []string{}, []string{}
	for _, r := range rows {
		appIDs, ids = append(appIDs, r.ApplicationID), append(ids, r.ID)
		userIDs = append(userIDs, r.InterviewerIds...)
	}
	apps, err := s.applications.Briefs(ctx, tx, appIDs)
	if err != nil {
		return nil, err
	}
	feedback, err := store.New(tx).ListFeedback(ctx, ids)
	if err != nil {
		return nil, err
	}
	byInterview := map[string][]FeedbackEntry{}
	for _, f := range feedback {
		byInterview[f.InterviewID] = append(byInterview[f.InterviewID], FeedbackEntry{
			UserID: f.UserID, Recommendation: f.Recommendation, Comments: f.Comments, SubmittedAt: f.SubmittedAt,
		})
		userIDs = append(userIDs, f.UserID)
	}
	names, err := s.users.UserNames(ctx, tx, userIDs)
	if err != nil {
		return nil, err
	}
	seesAll := p.Can(api.Permission("interviews:schedule"))
	now := time.Now()
	out := make([]View, len(rows))
	for i, r := range rows {
		d := detailsOf(r)
		all := byInterview[r.ID]
		visible, hidden := VisibleFeedback(all, p.UserID, d.Interviewers, seesAll)
		people := make([]Person, len(d.Interviewers))
		for j, id := range d.Interviewers {
			people[j] = Person{ID: id, Name: names[id]}
		}
		out[i] = View{
			ID: r.ID, Version: int(r.Version), App: apps[r.ApplicationID], Details: d, Interviewers: people,
			Feedback: visible, Names: names, FeedbackHidden: hidden, MyFeedbackDue: FeedbackDue(p.UserID, d, all, now),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, nil
}

func getRow(ctx context.Context, q *store.Queries, id string) (store.Interview, error) {
	row, err := q.GetInterview(ctx, id)
	if database.IsNoRows(err) {
		return row, apperr.NotFound("interview")
	}
	return row, err
}

func detailsOf(r store.Interview) Details {
	return Details{
		Title: r.Title, StartsAt: r.StartsAt, Duration: int(r.DurationMinutes), Mode: r.Mode, Location: r.Location,
		Instructions: r.Instructions, Interviewers: r.InterviewerIds, Status: r.Status,
	}
}

// fields are the audited fields; instructions only by length.
func fields(d Details) map[string]any {
	n := 0
	if d.Instructions != nil {
		n = len(*d.Instructions)
	}
	return map[string]any{
		"title": d.Title, "startsAt": d.StartsAt.UTC().Format(time.RFC3339), "durationMinutes": d.Duration, "mode": d.Mode,
		"location": d.Location, "instructionsLength": n, "interviewers": d.Interviewers, "status": d.Status,
	}
}
