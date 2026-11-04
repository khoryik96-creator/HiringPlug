package app

// Adapters giving the privacy module what it needs from other modules: names,
// counts, and a copy of everything held about a candidate (for access
// requests), in plain words, as the exporting user may see it.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

type privacyCandidates struct{ candidates *candidates.Module }

func (a privacyCandidates) Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	briefs, err := a.candidates.Briefs(ctx, tx, ids)
	out := make(map[string]string, len(briefs))
	for id, b := range briefs {
		out[id] = b.Name
	}
	return out, err
}

func (a privacyCandidates) Count(ctx context.Context, tx pgx.Tx) (int, error) {
	return a.candidates.Count(ctx, tx)
}

// privacyData gathers a candidate's data from each module. It looks the
// modules up when called: some are built after the privacy module.
type privacyData struct{ m *Modules }

func (a privacyData) About(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (privacy.Held, error) {
	v, err := a.m.Candidates.ViewIn(ctx, tx, p, id)
	if err != nil {
		return privacy.Held{}, err
	}
	pr := v.Profile
	held := privacy.Held{Name: strings.TrimSpace(pr.FirstName + " " + pr.LastName), NotIncluded: []string{}}
	held.Sections = append(held.Sections, profileSection(v), historySection(v.History), educationSection(v.History))
	if v.CompensationRestricted {
		held.NotIncluded = append(held.NotIncluded, "Salary (current and expected): you may not see it.")
	} else if s, ok := salarySection(v.Compensation); ok {
		held.Sections = append(held.Sections, s)
	}
	type part struct {
		perm   api.Permission
		what   string
		gather func() (privacy.Section, error)
	}
	for _, x := range []part{
		{api.PermissionDocumentsRead, "CVs and documents", func() (privacy.Section, error) { return a.documents(ctx, tx, id) }},
		{api.PermissionPipelineRead, "Jobs they were put forward for", func() (privacy.Section, error) { return a.applications(ctx, tx, id) }},
		{api.PermissionNotesRead, "Notes", func() (privacy.Section, error) {
			s, hidden, err := a.notes(ctx, tx, id)
			if hidden > 0 {
				held.NotIncluded = append(held.NotIncluded, fmt.Sprintf("%d sensitive note(s): you may not see them.", hidden))
			}
			return s, err
		}},
		{api.PermissionEmailsRead, "Emails", func() (privacy.Section, error) { return a.emails(ctx, tx, id) }},
	} {
		if !p.Can(x.perm) {
			held.NotIncluded = append(held.NotIncluded, x.what+": you may not see them.")
			continue
		}
		s, err := x.gather()
		if err != nil {
			return privacy.Held{}, err
		}
		held.Sections = append(held.Sections, s)
	}
	return held, nil
}

func profileSection(v candidates.View) privacy.Section {
	pr := v.Profile
	item := privacy.Item{{Label: "Name", Value: strings.TrimSpace(pr.FirstName + " " + pr.LastName)}}
	add := func(label string, s *string) {
		if s != nil && strings.TrimSpace(*s) != "" {
			item = append(item, privacy.Field{Label: label, Value: *s})
		}
	}
	add("Email", pr.Email)
	add("Phone", pr.Phone)
	add("Nationality", pr.Nationality)
	if w := pr.WorkAuthorization; w != nil {
		s := w.Status
		if w.Basis != nil {
			s += " (" + *w.Basis + ")"
		}
		add("Right to work", &s)
		add("Right to work note", w.Note)
	}
	if n := pr.NoticePeriod; n != nil {
		s := "immediately"
		if n.Value > 0 {
			s = fmt.Sprintf("%d %s(s)", n.Value, n.Unit)
		}
		add("Notice period", &s)
	}
	add("Current title", pr.CurrentTitle)
	add("Current employer", pr.CurrentCompany)
	add("Location", pr.Location)
	add("Industry", pr.Industry)
	add("Specialization", pr.Specialization)
	add("Where they came from", pr.Source)
	if len(pr.Skills) > 0 {
		s := strings.Join(pr.Skills, ", ")
		add("Skills", &s)
	}
	if len(pr.Tags) > 0 {
		s := strings.Join(pr.Tags, ", ")
		add("Tags", &s)
	}
	added := v.CreatedAt.Format(time.DateOnly)
	add("Added on", &added)
	return privacy.Section{Title: "Profile", Items: []privacy.Item{item}}
}

func historySection(h candidates.History) privacy.Section {
	s := privacy.Section{Title: "Work history", Items: []privacy.Item{}}
	for _, j := range h.Work {
		item := privacy.Item{}
		for _, f := range []struct {
			label string
			v     *string
		}{{"Title", j.Title}, {"Employer", j.Company}, {"From", j.StartDate}, {"To", j.EndDate}, {"Description", j.Description}} {
			if f.v != nil {
				item = append(item, privacy.Field{Label: f.label, Value: *f.v})
			}
		}
		if j.Current {
			item = append(item, privacy.Field{Label: "Current", Value: "yes"})
		}
		s.Items = append(s.Items, item)
	}
	return s
}

func educationSection(h candidates.History) privacy.Section {
	s := privacy.Section{Title: "Education", Items: []privacy.Item{}}
	for _, q := range h.Education {
		item := privacy.Item{}
		if q.Qualification != nil {
			item = append(item, privacy.Field{Label: "Qualification", Value: *q.Qualification})
		}
		if q.Institution != nil {
			item = append(item, privacy.Field{Label: "Institution", Value: *q.Institution})
		}
		if q.EndYear != nil {
			item = append(item, privacy.Field{Label: "Finished", Value: fmt.Sprint(*q.EndYear)})
		}
		s.Items = append(s.Items, item)
	}
	return s
}

func salarySection(c *candidates.Compensation) (privacy.Section, bool) {
	if c == nil || (c.Current == nil && c.Expected == nil) {
		return privacy.Section{}, false
	}
	item := privacy.Item{}
	if cur := c.Current; cur != nil {
		item = append(item, privacy.Field{Label: "Current salary", Value: fmt.Sprintf("%s %s a %s", cur.Currency, cur.Amount, cur.Period)})
	}
	if e := c.Expected; e != nil {
		v := e.Min
		if e.Max != nil {
			v += " to " + *e.Max
		}
		item = append(item, privacy.Field{Label: "Expected salary", Value: fmt.Sprintf("%s %s a %s", e.Currency, v, e.Period)})
	}
	return privacy.Section{Title: "Salary", Items: []privacy.Item{item}}, true
}

func (a privacyData) documents(ctx context.Context, tx pgx.Tx, id string) (privacy.Section, error) {
	docs, err := a.m.Documents.ForExport(ctx, tx, id)
	s := privacy.Section{Title: "CVs and documents", Items: []privacy.Item{}}
	for _, d := range docs {
		item := privacy.Item{{Label: "File", Value: d.FileName}, {Label: "Kind", Value: d.Source}, {Label: "Uploaded", Value: d.UploadedAt.Format(time.DateOnly)}}
		if d.Text != nil {
			item = append(item, privacy.Field{Label: "Text read from it", Value: *d.Text})
		}
		s.Items = append(s.Items, item)
	}
	return s, err
}

func (a privacyData) applications(ctx context.Context, tx pgx.Tx, id string) (privacy.Section, error) {
	apps, err := a.m.Pipeline.ForExport(ctx, tx, id)
	s := privacy.Section{Title: "Jobs they were put forward for", Items: []privacy.Item{}}
	for _, x := range apps {
		item := privacy.Item{
			{Label: "Job", Value: x.JobTitle}, {Label: "Employer", Value: x.CompanyName}, {Label: "Stage", Value: x.Stage},
			{Label: "Added", Value: x.AddedAt.Format(time.DateOnly)}, {Label: "Stage since", Value: x.StageChangedAt.Format(time.DateOnly)},
		}
		if x.Reason != nil {
			item = append(item, privacy.Field{Label: "Reason", Value: *x.Reason})
		}
		s.Items = append(s.Items, item)
	}
	return s, err
}

func (a privacyData) notes(ctx context.Context, tx pgx.Tx, id string) (privacy.Section, int, error) {
	notes, hidden, err := a.m.Notes.ForExport(ctx, tx, id)
	s := privacy.Section{Title: "Notes", Items: []privacy.Item{}}
	for _, n := range notes {
		s.Items = append(s.Items, privacy.Item{
			{Label: "Written", Value: n.CreatedAt.Format(time.DateOnly)}, {Label: "By", Value: n.AuthorName}, {Label: "Kind", Value: n.Kind}, {Label: "Note", Value: n.Body},
		})
	}
	return s, hidden, err
}

func (a privacyData) emails(ctx context.Context, tx pgx.Tx, id string) (privacy.Section, error) {
	emails, err := a.m.Emails.ForExport(ctx, tx, id)
	s := privacy.Section{Title: "Emails", Items: []privacy.Item{}}
	for _, e := range emails {
		when := e.CreatedAt
		if e.SentAt != nil {
			when = *e.SentAt
		}
		s.Items = append(s.Items, privacy.Item{
			{Label: "Date", Value: when.Format(time.DateOnly)}, {Label: "To", Value: strings.Join(e.To, ", ")}, {Label: "Subject", Value: e.Subject},
			{Label: "Status", Value: e.Status}, {Label: "Text", Value: e.Body},
		})
	}
	return s, err
}
