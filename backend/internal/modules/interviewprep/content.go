package interviewprep

// The AI's prep material, in the contract's shape, kept within its limits.

import (
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Prep is prep material the AI writes, or is writing.
type Prep struct {
	ID, JobID, Status   string // queued, done, failed
	Focus, Error, Model *string
	Content             *api.InterviewPrepContent
	QuestionCount       int
	Archived            bool
	CreatedAt           time.Time
	FinishedAt          *time.Time
}

// Short keeps the AI's material short and easy to read: each list and each
// line within its limits (the contract's), blank lines dropped.
func Short(c api.InterviewPrepContent) api.InterviewPrepContent {
	c.Summary = clip(c.Summary, 600)
	c.FocusAreas = lines(c.FocusAreas, 8, 160)
	c.QuestionsToAsk = lines(c.QuestionsToAsk, 6, 250)
	c.SoftSkills = items(c.SoftSkills, 10)
	c.Technical = items(c.Technical, 12)
	return c
}

func items(in []api.InterviewPrepItem, most int) []api.InterviewPrepItem {
	out := []api.InterviewPrepItem{}
	for _, it := range in {
		it.Question = clip(it.Question, 400)
		if it.Question == "" {
			continue
		}
		it.Kind, it.WhyAsked = clip(it.Kind, 60), clip(it.WhyAsked, 300)
		it.Approach, it.Phrasing = lines(it.Approach, 5, 200), lines(it.Phrasing, 3, 300)
		it.TradeOffs, it.Pitfalls = lines(it.TradeOffs, 4, 200), lines(it.Pitfalls, 4, 200)
		out = append(out, it)
		if len(out) == most {
			break
		}
	}
	return out
}
