package automations

// The automation rules, as plain functions on plain values: no database, no HTTP.

import (
	"strconv"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Triggers (WHEN).
const (
	ApplicationReceived = "application_received"
	StageChanged        = "stage_changed"
	InterviewScheduled  = "interview_scheduled"
	PlacementStarts     = "placement_starts"
	GuaranteeEnds       = "guarantee_ends"
	InvoiceDue          = "invoice_due"
)

// Actions (THEN).
const (
	EmailCandidate = "email_candidate"
	EmailClient    = "email_client"
	EmailRecruiter = "email_recruiter"
	Task           = "task"
)

// Send modes, and run statuses.
const (
	SendOnTheirOwn = "send"
	AskFirst       = "approve"

	Scheduled = "scheduled"
	ToApprove = "to_approve"
	Done      = "done"
	Skipped   = "skipped"
	Failed    = "failed"
)

// DayHour is when an automation counted in days from a date (a start date,
// a guarantee's end, an invoice's due date) runs: 9 in the morning.
const DayHour = 9

var stages = map[string]bool{"applied": true, "screening": true, "submitted": true, "interview": true, "offer": true, "placed": true, "rejected": true, "withdrawn": true}

// Action is one thing an automation does.
type Action struct {
	Kind          string
	Subject, Body string // emails
	Title         string // tasks
}

// IsEmail reports whether the action sends an email.
func (a Action) IsEmail() bool { return a.Kind != Task }

// Rule is an automation: WHEN Trigger (in Stage), IF at CompanyID, THEN
// after OffsetDays the Actions.
type Rule struct {
	Name       string
	Enabled    bool
	Trigger    string
	Stage      *string
	OffsetDays int
	CompanyID  *string
	Actions    []Action
	SendMode   string
}

// HasEmail reports whether the rule sends an email.
func (r Rule) HasEmail() bool {
	for _, a := range r.Actions {
		if a.IsEmail() {
			return true
		}
	}
	return false
}

// dated reports whether a trigger counts from a date that can be ahead (so
// days before it make sense), rather than from the moment it happens.
func dated(trigger string) bool {
	return trigger != ApplicationReceived && trigger != StageChanged
}

// NormalizeRule tidies an automation and checks it.
func NormalizeRule(r Rule) (Rule, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len([]rune(r.Name)) > 120 {
		errs.Add("name", "enter a name of at most 120 characters")
	}
	switch r.Trigger {
	case StageChanged:
		if r.Stage == nil || !stages[*r.Stage] {
			errs.Add("stage", "choose the stage")
		}
	case ApplicationReceived, InterviewScheduled, PlacementStarts, GuaranteeEnds, InvoiceDue:
		r.Stage = nil
	default:
		errs.Add("trigger", "choose when it starts")
	}
	switch {
	case r.OffsetDays < -60 || r.OffsetDays > 365:
		errs.Add("offsetDays", "must be between 60 days before and 365 days after")
	case r.OffsetDays < 0 && !dated(r.Trigger):
		errs.Add("offsetDays", "can only be after this (it has no date ahead)")
	}
	if r.SendMode != SendOnTheirOwn && r.SendMode != AskFirst {
		errs.Add("sendMode", "must be send or approve")
	}
	if len(r.Actions) == 0 || len(r.Actions) > 3 {
		errs.Add("actions", "one to three things to do")
	}
	for i, a := range r.Actions {
		at := func(f string) string { return "actions." + strconv.Itoa(i) + "." + f }
		a.Subject, a.Body, a.Title = strings.TrimSpace(a.Subject), strings.TrimSpace(a.Body), strings.TrimSpace(a.Title)
		switch a.Kind {
		case EmailCandidate, EmailClient, EmailRecruiter:
			a.Title = ""
			if a.Subject == "" || len([]rune(a.Subject)) > 300 {
				errs.Add(at("subject"), "enter a subject")
			}
			if a.Body == "" || len([]rune(a.Body)) > 10000 {
				errs.Add(at("body"), "enter the message")
			}
		case Task:
			a.Subject, a.Body = "", ""
			if a.Title == "" || len([]rune(a.Title)) > 200 {
				errs.Add(at("title"), "say what to do")
			}
		default:
			errs.Add(at("kind"), "choose what to do")
		}
		r.Actions[i] = a
	}
	return r, errs
}

// DueAt is when a run is due: the moment (an application arriving, a stage
// change, an interview's time) plus the days, or a date (a start, a
// guarantee's end, an invoice's due date) plus the days, at DayHour there.
func DueAt(r Rule, anchor time.Time, loc *time.Location) time.Time {
	switch r.Trigger {
	case PlacementStarts, GuaranteeEnds, InvoiceDue:
		y, m, d := anchor.Date()
		return time.Date(y, m, d+r.OffsetDays, DayHour, 0, 0, 0, loc)
	default:
		return anchor.AddDate(0, 0, r.OffsetDays)
	}
}

// When decides when a run goes, given when it is due and when what it is
// about happens (an interview, a start): on time; now, if it is a little late
// (up to an hour) or late but before what it is about; not at all otherwise
// (a reminder for something already past).
func When(due, about, now time.Time) (time.Time, bool) {
	switch {
	case !due.Before(now):
		return due, true
	case !due.Before(now.Add(-time.Hour)), about.After(now):
		return now, true
	default:
		return time.Time{}, false
	}
}

// FillTitle fills a task's {{fields}} from values (unknown ones stay as written).
func FillTitle(title string, values map[string]string) string {
	for k, v := range values {
		title = strings.ReplaceAll(title, "{{"+k+"}}", v)
	}
	return title
}

// Preset is a ready-made automation.
type Preset struct {
	Key string
	Rule
}

func email(kind, subject, body string) Action {
	return Action{Kind: kind, Subject: subject, Body: body}
}
func task(title string) Action { return Action{Kind: Task, Title: title} }

// Presets are the ready-made automations, added (switched off) the first time
// a company opens its automations.
func Presets(sendMode string) []Preset {
	rejected := "rejected"
	p := func(key, name, trigger string, stage *string, offset int, actions ...Action) Preset {
		return Preset{Key: key, Rule: Rule{Name: name, Trigger: trigger, Stage: stage, OffsetDays: offset, Actions: actions, SendMode: sendMode}}
	}
	sign := "\n\nBest regards,\n{{sender.name}}\n{{agency.name}}"
	return []Preset{
		p("application_received", "Application received", ApplicationReceived, nil, 0,
			email(EmailCandidate, "Thank you for applying: {{job.title}}",
				"Dear {{candidate.firstName}},\n\nThank you for your interest in the {{job.title}} role. We have your details and will be in touch if your experience is a match."+sign)),
		p("interview_reminder", "Interview reminder, the day before", InterviewScheduled, nil, -1,
			email(EmailCandidate, "Reminder: your interview for {{job.title}}",
				"Dear {{candidate.firstName}},\n\nA reminder of your interview for the {{job.title}} role: {{interview.when}} ({{interview.how}}).\n\n{{interview.instructions}}\n\nGood luck!"+sign)),
		p("not_progressing", "Not progressing, 2 days after", StageChanged, &rejected, 2,
			email(EmailCandidate, "Your application for {{job.title}}",
				"Dear {{candidate.firstName}},\n\nThank you for your time and interest in the {{job.title}} role. After careful consideration, we will not be taking your application further this time. We will keep your details and be in touch about other roles that suit you."+sign)),
		p("checkin_30", "Placement check-in, 30 days", PlacementStarts, nil, 30,
			email(EmailCandidate, "How are things going?",
				"Dear {{candidate.firstName}},\n\nIt has been a month since you started as {{job.title}}. How are you settling in? I would be glad to hear how it is going."+sign),
			task("30-day check-in: call {{candidate.fullName}} ({{job.title}})")),
		p("checkin_90", "Placement check-in, 90 days", PlacementStarts, nil, 90,
			email(EmailCandidate, "Three months in",
				"Dear {{candidate.firstName}},\n\nCongratulations on three months as {{job.title}}! I hope it is going well. Let me know if there is anything I can help with."+sign),
			task("90-day check-in: call {{candidate.fullName}} ({{job.title}})")),
		p("guarantee_ending", "Guarantee ending, a week before", GuaranteeEnds, nil, -7,
			task("Guarantee ends in a week: check in with {{candidate.fullName}} and {{company.name}}")),
		p("invoice_overdue", "Invoice overdue", InvoiceDue, nil, 1,
			task("Invoice overdue: chase {{company.name}} for {{candidate.fullName}}'s placement")),
	}
}
