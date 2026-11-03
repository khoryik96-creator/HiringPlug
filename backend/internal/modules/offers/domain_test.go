package offers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
)

func ptr[T any](v T) *T { return &v }

var now = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

func TestApprovalPath(t *testing.T) {
	o := Offer{Status: Draft, StartDate: ptr(now.AddDate(0, 1, 0))}

	_, problem := Step(o, StepRequestApproval, "usr_rec", false, false, now)
	assert.Contains(t, problem, "salary", "nothing goes for approval without a salary")
	_, problem = Step(o, StepSend, "usr_rec", false, true, now)
	assert.Contains(t, problem, "Ask for approval first")

	o, problem = Step(o, StepRequestApproval, "usr_rec", false, true, now)
	assert.Empty(t, problem)
	assert.Equal(t, PendingApproval, o.Status)
	_, problem = Decide(o, true, nil, "usr_rec", now)
	assert.Contains(t, problem, "someone else approves it")

	back, problem := Decide(o, false, ptr("  Too high for the band "), "usr_hm", now)
	assert.Empty(t, problem)
	assert.Equal(t, Draft, back.Status, "sent back")
	assert.Equal(t, "Too high for the band", *back.DecisionNote)

	o, _ = Decide(o, true, nil, "usr_hm", now)
	assert.Equal(t, Approved, o.Status)
	o, problem = Step(o, StepSend, "usr_rec", false, true, now)
	assert.Empty(t, problem)
	assert.Equal(t, Sent, o.Status)
	assert.Equal(t, "usr_hm", *o.DecidedBy, "approved by the approver")

	_, problem = Edit(o, TermsPatch{Terms: apiconv.Change[*string]{Set: true, Value: ptr("x")}})
	assert.Contains(t, problem, "withdraw it")
	o, _ = Step(o, StepAccept, "usr_rec", false, true, now)
	assert.Equal(t, Accepted, o.Status)
	assert.Equal(t, now, *o.AnsweredAt)
	_, problem = Step(o, StepWithdraw, "usr_rec", false, true, now)
	assert.NotEmpty(t, problem)
}

func TestApproversSendStraightAway(t *testing.T) {
	o, problem := Step(Offer{Status: Draft, StartDate: ptr(now)}, StepSend, "usr_owner", true, true, now)
	assert.Empty(t, problem)
	assert.Equal(t, Sent, o.Status)
	assert.Equal(t, "usr_owner", *o.DecidedBy, "sending is their approval")
}

func TestChangingAnApprovedOfferNeedsApprovalAgain(t *testing.T) {
	o := Offer{Status: Approved, DecidedBy: ptr("usr_hm"), DecidedAt: &now}
	o, problem := Edit(o, TermsPatch{StartDate: apiconv.Change[*time.Time]{Set: true, Value: ptr(now.AddDate(0, 2, 0))}})
	assert.Empty(t, problem)
	assert.Equal(t, Draft, o.Status)
	assert.Nil(t, o.DecidedBy)
}
