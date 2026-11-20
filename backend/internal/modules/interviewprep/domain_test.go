package interviewprep

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func TestAQuestionIsTidiedAndChecked(t *testing.T) {
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	blank, source := "  ", " Ali, 2nd round "
	f, errs := Clean(QuestionFields{Question: "  How would   you scale\tthe pipeline? ", Kind: "technical", Notes: &blank, Source: &source}, today)
	assert.NoError(t, errs.Err())
	assert.Equal(t, "How would you scale the pipeline?", f.Question)
	assert.Nil(t, f.Notes)
	assert.Equal(t, "Ali, 2nd round", *f.Source)

	tomorrow := today.AddDate(0, 0, 1)
	_, errs = Clean(QuestionFields{Question: " ", Kind: "other", AskedOn: &tomorrow}, today)
	assert.Equal(t, []string{"question", "askedOn"}, []string{errs[0].Field, errs[1].Field})
}

func TestTheAIIsToldTheJobWithoutItsSalary(t *testing.T) {
	text := JobText("Lead the data platform team.\nSalary: RM 12,000 - 15,000 monthly\nPackage up to SGD 9k plus bonus",
		"Senior Data Engineer\n\nLead the data platform team.")
	assert.Contains(t, text, "Lead the data platform team.")
	assert.Contains(t, text, "Senior Data Engineer")
	assert.NotContains(t, text, "12,000")
	assert.NotContains(t, text, "SGD")
	assert.Equal(t, 1, strings.Count(text, "Lead the data platform team."), "the ad's copy of the description only once")
	assert.Len(t, []rune(JobText(strings.Repeat("a", MaxJobText+50), "")), MaxJobText)
}

func TestThereMustBeSomethingToPrepareFrom(t *testing.T) {
	assert.False(t, EnoughToPrepare("Engineer", 0))
	assert.True(t, EnoughToPrepare("Engineer", 1))
	assert.True(t, EnoughToPrepare(strings.Repeat("Build and run the data platform. ", 2), 0))
}

func TestPrepStuckInTheQueueShowsAsFailed(t *testing.T) {
	at := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	p := Prep{Status: "queued", CreatedAt: at}
	assert.Equal(t, "queued", Shown(p, at.Add(10*time.Minute)).Status)
	late := Shown(p, at.Add(31*time.Minute))
	assert.Equal(t, "failed", late.Status)
	assert.Contains(t, *late.Error, "did not answer")
}

func TestTheMaterialIsKeptShort(t *testing.T) {
	long := strings.Repeat("word ", 200)
	steps := []string{"Clarify the goal", " ", "Name the constraint", "Pick an option", "Say the trade-off", "Close with the result", "One too many"}
	c := Short(api.InterviewPrepContent{
		Summary: long, FocusAreas: []string{"Data quality", ""},
		SoftSkills: []api.InterviewPrepItem{{Question: "What if a stakeholder disagrees?", WhyAsked: long, Approach: steps, Phrasing: []string{"I'd start by…"}}, {Question: "  "}},
	})
	assert.LessOrEqual(t, len([]rune(c.Summary)), 600)
	assert.True(t, strings.HasSuffix(c.Summary, "…"))
	assert.Equal(t, []string{"Data quality"}, c.FocusAreas)
	assert.Len(t, c.SoftSkills, 1, "an item without a question is dropped")
	assert.Equal(t, []string{"Clarify the goal", "Name the constraint", "Pick an option", "Say the trade-off", "Close with the result"}, c.SoftSkills[0].Approach)
	assert.LessOrEqual(t, len([]rune(c.SoftSkills[0].WhyAsked)), 300)
	assert.Empty(t, c.Technical)
}
