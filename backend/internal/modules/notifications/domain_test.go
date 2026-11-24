package notifications

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kl(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	return loc
}

func TestNextMorning(t *testing.T) {
	loc := kl(t)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 22, h, m, 0, 0, loc) }
	assert.Equal(t, at(8, 0), NextMorning(at(7, 59), loc), "before 8: this morning")
	assert.Equal(t, at(8, 0).AddDate(0, 0, 1), NextMorning(at(8, 0), loc), "at 8: tomorrow (strictly after)")
	assert.Equal(t, at(8, 0).AddDate(0, 0, 1), NextMorning(at(23, 30), loc))
	assert.Equal(t, at(8, 0), NextMorning(time.Date(2026, 11, 21, 23, 0, 0, 0, time.UTC), loc), "23:00 UTC is 7:00 in Kuala Lumpur")
}

func TestBuild(t *testing.T) {
	loc := kl(t)
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, loc)
	w := WindowAt(now, loc)
	assert.Equal(t, time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), w.Today)
	assert.Equal(t, time.Date(2026, 11, 24, 0, 0, 0, 0, loc), w.To, "to the end of tomorrow")

	appID := "app_1"
	in := Inputs{
		Apps:       map[string]Application{appID: {CandidateID: "cand_1", CandidateName: "Siti", JobTitle: "Accountant", CompanyName: "Contoh"}},
		Applicants: []Record{{ID: appID, ApplicationID: appID, At: now.Add(-time.Hour)}, {ID: "gone", ApplicationID: "gone", At: now}},
		Tasks: []Task{
			{ID: "t1", Title: "Call back", DueOn: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), ApplicationID: &appID, CreatedAt: now.AddDate(0, 0, -5)},
			{ID: "t2", Title: "Send CVs", DueOn: w.Today, CreatedAt: now.AddDate(0, 0, -1)},
		},
		Interviews: []Interview{{ID: "i1", ApplicationID: appID, Title: "First", StartsAt: time.Date(2026, 11, 23, 14, 30, 0, 0, loc), CreatedAt: now.AddDate(0, 0, -3)}},
	}
	seen := now.Add(-30 * time.Minute)
	items, unseen := Shown(Build(in, &seen, w))
	require.Len(t, items, 4, "the applicant whose application is hidden is left out")
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	assert.Equal(t, []string{"applicant:app_1", "interview:i1", "task:t2", "task:t1"}, ids,
		"newest first: an interview counts from the day before, a task from its due day")
	assert.Equal(t, "Overdue, due Fri 20 Nov · Siti, Accountant", find(items, "task:t1").Detail)
	assert.Equal(t, "#/candidates/cand_1", find(items, "task:t1").Href)
	assert.Equal(t, "Due today", find(items, "task:t2").Detail)
	assert.Equal(t, "#/", find(items, "task:t2").Href, "a task without a candidate opens the dashboard")
	assert.Equal(t, "Interview tomorrow, 2:30 PM: Siti", find(items, "interview:i1").Title)
	assert.Equal(t, "First · Accountant, Contoh", find(items, "interview:i1").Detail)
	assert.Zero(t, unseen, "all before seen")

	_, unseen = Shown(Build(in, nil, w))
	assert.Equal(t, 4, unseen, "never looked: all new")
}

func find(items []Item, id string) Item {
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	return Item{}
}

func TestDigest(t *testing.T) {
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	items := []Item{
		{Kind: KindApplicant, Title: "Nur applied", Detail: "Clerk", Href: "#/candidates/c1", At: now.Add(-2 * time.Hour)},
		{Kind: KindApplicant, Title: "Old applied", Detail: "Clerk", Href: "#/candidates/c2", At: now.Add(-48 * time.Hour)},
		{Kind: KindTask, Title: "Call back", Detail: "Due today", Href: "#/"},
		{Kind: KindTask, Title: "Send CVs", Detail: "Due today", Href: "#/"},
	}
	picked := DigestFor(items, DigestSince(time.Time{}, now, time.UTC))
	assert.Len(t, picked, 3, "applicants already in yesterday's email are left out")
	subject, body, ok := DigestText(picked, "https://hp.example/")
	require.True(t, ok)
	assert.Equal(t, "Today: 2 tasks due, 1 new applicant", subject)
	assert.Contains(t, body, "- Nur applied (Clerk)\n  https://hp.example/#/candidates/c1")
	assert.Less(t, strings.Index(body, "Tasks due"), strings.Index(body, "New applicants"), "tasks before applicants")
	_, _, ok = DigestText(nil, "https://hp.example")
	assert.False(t, ok, "nothing to say: no email")
}

func TestNormalizeMuted(t *testing.T) {
	got, errs := NormalizeMuted([]string{KindInterview, KindApplicant})
	assert.NoError(t, errs.Err())
	assert.Equal(t, []string{KindApplicant, KindInterview}, got)
	_, errs = NormalizeMuted([]string{"birthdays"})
	assert.Error(t, errs.Err())
	assert.Equal(t, []string{}, NormalizeMutedSilently(nil), "never null")
}

func TestDigestSince(t *testing.T) {
	loc := kl(t)
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, loc)
	assert.Equal(t, now.Add(-24*time.Hour), DigestSince(time.Time{}, now, loc), "the first email: the last day")
	assert.Equal(t, time.Date(2026, 11, 22, 8, 0, 0, 0, loc), DigestSince(time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), now, loc),
		"since the last email's morning: a missed morning loses no applicant")
}

func TestNextDayMorning(t *testing.T) {
	loc := kl(t)
	assert.Equal(t, time.Date(2026, 11, 23, 8, 0, 0, 0, loc), NextDayMorning(time.Date(2026, 11, 22, 7, 59, 58, 0, loc), loc),
		"a clock a little behind 8:00 still queues tomorrow, not this morning again")
}

func TestShownCapsTheBellNotTheEmail(t *testing.T) {
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	var items []Item
	for i := range MaxItems + 5 {
		items = append(items, Item{ID: fmt.Sprint(i), Kind: KindApplicant, At: now.Add(-time.Duration(i) * time.Minute), New: true})
	}
	items = append(items, Item{ID: "iv", Kind: KindInterview, At: now.Add(-48 * time.Hour)})
	shown, unseen := Shown(items)
	assert.Len(t, shown, MaxItems)
	assert.Equal(t, MaxItems, unseen)
	assert.Equal(t, "iv", DigestFor(items, now.Add(-24*time.Hour))[MaxItems+5].ID, "the email is picked from all, so the interview is in it")
}

func TestAFormattedCVIsForTheBellNotTheMorningEmail(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	items := Build(Inputs{FormattedCVs: []FormattedCV{
		{ID: "scv_1", CandidateID: "cand_1", CandidateName: "Siti Aisyah", Blind: true, At: now.Add(-time.Minute)},
		{ID: "scv_2", CandidateID: "cand_2", CandidateName: "Lim Wei", Failed: true, At: now.Add(-2 * time.Minute)},
	}}, nil, WindowAt(now, time.UTC))
	require.Len(t, items, 2)
	assert.Equal(t, "Blind CV formatted: Siti Aisyah", items[0].Title)
	assert.Equal(t, "#/candidates/cand_1", items[0].Href)
	assert.Equal(t, "CV could not be formatted: Lim Wei", items[1].Title)
	assert.Empty(t, DigestFor(items, now.Add(-24*time.Hour)))
}

func TestAILimitShowsOneItemPerMonthTheLimitOverTheWarning(t *testing.T) {
	loc := kl(t)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, loc)
	nov, dec := time.Date(2026, 11, 1, 0, 0, 0, 0, loc), time.Date(2026, 12, 1, 0, 0, 0, 0, loc)
	warned, reached := now.Add(-3*time.Hour), now.Add(-time.Hour)
	w := WindowAt(now, loc)

	items := Build(Inputs{AILimits: []AILimit{{Level: 80, At: warned, Month: nov, Until: dec}}}, nil, w)
	require.Len(t, items, 1)
	assert.Equal(t, Item{ID: "ai_limit:2026-11:80", Kind: KindAILimit, Title: "AI use reached 80% of this month's limit",
		Detail: "AI features pause at 100%, until 1 Dec", Href: "#/settings/ai-usage", At: warned, New: true}, items[0])

	seen := now.Add(-2 * time.Hour)
	items = Build(Inputs{AILimits: []AILimit{
		{Level: 80, At: warned, Month: nov, Until: dec}, {Level: AILimitLevel, At: reached, Month: nov, Until: dec},
	}}, &seen, w)
	require.Len(t, items, 1, "once the limit is reached, only that shows")
	assert.Equal(t, "ai_limit:2026-11:100", items[0].ID)
	assert.Equal(t, "AI limit reached: AI features are paused until 1 Dec", items[0].Title)
	assert.True(t, items[0].New, "reached after the user last looked")

	picked := DigestFor(items, now.Add(-24*time.Hour))
	subject, body, ok := DigestText(picked, "https://hp.example")
	require.True(t, ok)
	assert.Equal(t, "Today: 1 AI limit notice", subject)
	assert.Contains(t, body, "The company's AI use this month\n- AI limit reached: AI features are paused until 1 Dec")
	assert.Contains(t, body, "https://hp.example/#/settings/ai-usage")
	assert.Empty(t, DigestFor(items, now), "a notice already in an earlier email is not sent again")
}

func TestEveryKindHasAMorningEmailSectionOrIsLeftOut(t *testing.T) {
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	for _, kind := range Kinds {
		picked := DigestFor([]Item{{Kind: kind, Title: "x", At: now}}, now.Add(-time.Hour))
		subject, _, ok := DigestText(picked, "https://hp.example")
		if len(picked) > 0 {
			assert.True(t, ok, kind)
			assert.NotEqual(t, "Today: ", subject, "%s needs a section and a count", kind)
		}
	}
}
