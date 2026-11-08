package jobs

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func TestARewriteFitsTheBlindAdsLimits(t *testing.T) {
	long := strings.Repeat("Keep a large sorting hub running smoothly, every shift. ", 35) // ~2000 characters
	items := make([]string, 60)
	for i := range items {
		items[i] = long
	}
	jd := api.BlindJd{JobTitle: strings.Repeat("Hub Manager ", 30), AboutClient: strings.Repeat("A regional group. ", 200),
		Responsibilities: items, Requirements: items, NiceToHave: items, WhyJoin: items}
	title, text := RewriteText(jd, "Hub Manager")
	assert.LessOrEqual(t, utf8.RuneCountInString(title), MaxBlindTitle)
	assert.LessOrEqual(t, utf8.RuneCountInString(text), MaxBlindText, "a person's edit could not be longer either")
	assert.True(t, strings.HasSuffix(text, "every shift."), "cut at a whole line: %q", text[len(text)-40:])

	_, short := RewriteText(api.BlindJd{JobTitle: "Clerk", AboutClient: "A firm."}, "Clerk")
	assert.Equal(t, "About our client\nA firm.", short, "short ones are kept whole")
}

func TestARewriteTheAINeverAnsweredIsShownAsFailed(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	runs := expireStale([]BlindRewrite{
		{ID: "fresh", Status: "queued", CreatedAt: now.Add(-5 * time.Minute)},
		{ID: "lost", Status: "queued", CreatedAt: now.Add(-RewriteGivesUpAfter - time.Minute)},
		{ID: "done", Status: "done", CreatedAt: now.Add(-48 * time.Hour)},
	}, now)
	assert.Equal(t, "queued", runs[0].Status, "still being written")
	assert.Equal(t, "failed", runs[1].Status, "so the ad can be rewritten again")
	assert.Contains(t, *runs[1].Error, "took too long")
	assert.Equal(t, "done", runs[2].Status)
	assert.Nil(t, runs[2].Error)
}
