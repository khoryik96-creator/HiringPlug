package interviewprep

// The AI's prep material, in the contract's shape, kept within its limits.

import (
	"strings"
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
	const limit = 14
	recorded := 0
	for _, group := range [][]api.InterviewPrepItem{c.SoftSkills, c.Technical} {
		for _, item := range group {
			if item.FromInterviews && strings.TrimSpace(item.Question) != "" {
				recorded++
			}
		}
	}
	remaining := map[bool]int{true: min(recorded, limit), false: max(0, limit-recorded)}
	c.SoftSkills = items(c.SoftSkills, remaining)
	c.Technical = items(c.Technical, remaining)
	return c
}

func items(in []api.InterviewPrepItem, remaining map[bool]int) []api.InterviewPrepItem {
	out := []api.InterviewPrepItem{}
	for _, it := range in {
		it.Question = clip(it.Question, 400)
		if it.Question == "" || remaining[it.FromInterviews] == 0 {
			continue
		}
		it.Kind, it.WhyAsked = clip(it.Kind, 60), clip(it.WhyAsked, 300)
		it.Approach, it.Phrasing = lines(it.Approach, 5, 200), lines(it.Phrasing, 3, 300)
		it.TradeOffs, it.Pitfalls = lines(it.TradeOffs, 4, 200), lines(it.Pitfalls, 4, 200)
		out = append(out, it)
		remaining[it.FromInterviews]--
	}
	return out
}
