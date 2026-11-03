package offers

// The offer rules, as plain functions on plain values: no database, no HTTP.
// Each returns the changed offer, or a problem in words for the user.

import (
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
)

// Statuses, in order.
const (
	Draft           = "draft"
	PendingApproval = "pending_approval"
	Approved        = "approved"
	Sent            = "sent"
	Accepted        = "accepted"
	Declined        = "declined"
	Withdrawn       = "withdrawn"
)

// Steps that move an offer on (besides an approver's decision).
const (
	StepRequestApproval = "request_approval"
	StepSend            = "send"
	StepAccept          = "accept"
	StepDecline         = "decline"
	StepWithdraw        = "withdraw"
)

// Offer is an offer's state and terms (all but the salary).
type Offer struct {
	Status                 string
	StartDate, ExpiresOn   *time.Time
	Terms                  *string
	RequestedBy, DecidedBy *string
	DecidedAt              *time.Time
	DecisionNote           *string
	SentAt, AnsweredAt     *time.Time
}

// TermsPatch is a partial change of an offer's terms.
type TermsPatch struct {
	StartDate apiconv.Change[*time.Time]
	ExpiresOn apiconv.Change[*time.Time]
	Terms     apiconv.Change[*string]
}

// Edit applies a change of terms. A sent or answered offer cannot change
// (withdraw it and make a new one); one waiting for approval or approved goes
// back to draft, since what was approved is no longer what will be sent.
func Edit(o Offer, p TermsPatch) (Offer, string) {
	switch o.Status {
	case Sent, Accepted, Declined, Withdrawn:
		return o, "A sent or finished offer cannot be changed: withdraw it and make a new one."
	case PendingApproval, Approved:
		o.Status, o.DecidedBy, o.DecidedAt = Draft, nil, nil
	}
	apiconv.Apply(&o.StartDate, p.StartDate)
	apiconv.Apply(&o.ExpiresOn, p.ExpiresOn)
	apiconv.Apply(&o.Terms, p.Terms)
	o.Terms = trimmed(o.Terms)
	return o, ""
}

// Step moves an offer on:
//   - request_approval: a draft waits for an approver;
//   - send: an approved offer, or a draft when the user may approve offers
//     (sending is then their approval);
//   - accept, decline: the candidate's answer to a sent offer;
//   - withdraw: any offer not yet answered.
//
// Asking for approval or sending needs a start date and a salary.
func Step(o Offer, step, user string, canApprove, hasSalary bool, now time.Time) (Offer, string) {
	ready := func() string {
		if o.StartDate == nil || !hasSalary {
			return "Set the salary and the start date first."
		}
		return ""
	}
	switch step {
	case StepRequestApproval:
		if o.Status != Draft {
			return o, "Only a draft can be sent for approval."
		}
		if problem := ready(); problem != "" {
			return o, problem
		}
		o.Status, o.RequestedBy, o.DecidedBy, o.DecidedAt, o.DecisionNote = PendingApproval, &user, nil, nil, nil
	case StepSend:
		switch {
		case o.Status == Draft && !canApprove:
			return o, "Ask for approval first: only approvers send offers without it."
		case o.Status == PendingApproval:
			return o, "It is waiting for approval."
		case o.Status != Draft && o.Status != Approved:
			return o, "This offer has already been sent."
		}
		if problem := ready(); problem != "" {
			return o, problem
		}
		if o.Status == Draft { // sent by an approver: approved by sending
			o.DecidedBy, o.DecidedAt = &user, &now
		}
		o.Status, o.SentAt = Sent, &now
	case StepAccept, StepDecline:
		if o.Status != Sent {
			return o, "Only a sent offer can be accepted or declined."
		}
		o.Status, o.AnsweredAt = Declined, &now
		if step == StepAccept {
			o.Status = Accepted
		}
	case StepWithdraw:
		if o.Status == Accepted || o.Status == Declined || o.Status == Withdrawn {
			return o, "This offer is already answered or withdrawn."
		}
		o.Status = Withdrawn
	default:
		return o, "Unknown step."
	}
	return o, ""
}

// Decide is an approver's decision on an offer waiting for approval: approve
// it, or send it back to draft (with a note why). Nobody decides on their own
// request.
func Decide(o Offer, approve bool, note *string, user string, now time.Time) (Offer, string) {
	if o.Status != PendingApproval {
		return o, "This offer is not waiting for approval."
	}
	if o.RequestedBy != nil && *o.RequestedBy == user {
		return o, "You asked for this approval: someone else approves it."
	}
	o.Status = Draft
	if approve {
		o.Status = Approved
	}
	o.DecidedBy, o.DecidedAt, o.DecisionNote = &user, &now, trimmed(note)
	return o, ""
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
