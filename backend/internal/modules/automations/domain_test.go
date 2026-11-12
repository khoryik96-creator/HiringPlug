package automations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestNormalizeRule(t *testing.T) {
	r, errs := NormalizeRule(Rule{Name: " Thank you ", Trigger: StageChanged, Stage: ptr("rejected"), OffsetDays: 2, SendMode: AskFirst,
		Actions: []Action{{Kind: EmailCandidate, Subject: " Hi ", Body: "Body", Title: "dropped"}, {Kind: Task, Title: " Call ", Subject: "dropped"}}})
	require.NoError(t, errs.Err())
	assert.Equal(t, "Thank you", r.Name)
	assert.Equal(t, Action{Kind: EmailCandidate, Subject: "Hi", Body: "Body"}, r.Actions[0])
	assert.Equal(t, Action{Kind: Task, Title: "Call"}, r.Actions[1])
	assert.True(t, r.HasEmail())

	r, errs = NormalizeRule(Rule{Name: "x", Trigger: PlacementStarts, Stage: ptr("rejected"), OffsetDays: -3, SendMode: SendOnTheirOwn, Actions: []Action{{Kind: Task, Title: "t"}}})
	require.NoError(t, errs.Err())
	assert.Nil(t, r.Stage, "only a stage change has a stage")
	assert.False(t, r.HasEmail())

	_, errs = NormalizeRule(Rule{Trigger: StageChanged, OffsetDays: -1, SendMode: "later", Actions: []Action{{Kind: EmailClient}, {Kind: "call"}}})
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, f := range []string{"name", "stage", "offsetDays", "sendMode", "actions.0.subject", "actions.0.body", "actions.1.kind"} {
		assert.True(t, fields[f], f)
	}
	_, errs = NormalizeRule(Rule{Name: "x", Trigger: "whenever", SendMode: AskFirst})
	assert.Len(t, errs, 2, "a trigger and something to do")
}

func TestDueAt(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	at := time.Date(2026, 12, 1, 15, 30, 0, 0, kl)
	assert.Equal(t, at.AddDate(0, 0, 2), DueAt(Rule{Trigger: StageChanged, OffsetDays: 2}, at, kl), "from the moment")
	assert.Equal(t, at.AddDate(0, 0, -1), DueAt(Rule{Trigger: InterviewScheduled, OffsetDays: -1}, at, kl), "a day before the interview's time")
	start := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC) // a day
	assert.Equal(t, time.Date(2026, 12, 31, 9, 0, 0, 0, kl), DueAt(Rule{Trigger: PlacementStarts, OffsetDays: 30}, start, kl), "at 9 in the morning there")
	assert.Equal(t, time.Date(2026, 11, 24, 9, 0, 0, 0, kl), DueAt(Rule{Trigger: GuaranteeEnds, OffsetDays: -7}, start, kl))
}

func TestWhen(t *testing.T) {
	now := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	got, ok := When(now.Add(time.Hour), now.Add(25*time.Hour), now)
	assert.True(t, ok)
	assert.Equal(t, now.Add(time.Hour), got, "on time")
	got, ok = When(now.Add(-5*time.Hour), now.Add(19*time.Hour), now)
	assert.True(t, ok)
	assert.Equal(t, now, got, "booked late: the reminder goes now, before the interview")
	got, ok = When(now.Add(-time.Minute), now.Add(-time.Minute), now)
	assert.True(t, ok)
	assert.Equal(t, now, got, "a moment late: now")
	_, ok = When(now.AddDate(0, 0, -30), now.AddDate(0, 0, -60), now)
	assert.False(t, ok, "a check-in for a placement recorded long after it started: not at all")
}

func TestPresets(t *testing.T) {
	presets := Presets(AskFirst)
	assert.Len(t, presets, 7)
	keys := map[string]bool{}
	for _, p := range presets {
		r, errs := NormalizeRule(p.Rule)
		require.NoError(t, errs.Err(), p.Key)
		assert.Equal(t, AskFirst, r.SendMode)
		assert.False(t, keys[p.Key], "unique keys")
		keys[p.Key] = true
	}
	assert.Equal(t, "Call Siti", FillTitle("Call {{candidate.firstName}}", map[string]string{"candidate.firstName": "Siti"}))
}
