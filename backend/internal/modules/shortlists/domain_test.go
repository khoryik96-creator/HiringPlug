package shortlists

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestNormalizeInput(t *testing.T) {
	in, days, errs := NormalizeInput(Input{Picks: []Pick{{ApplicationID: "a", Note: ptr("  Good.  ")}, {ApplicationID: "b", Note: ptr(" ")}}, Message: ptr("  ")})
	require.Empty(t, errs)
	assert.Equal(t, DefaultDays, days, "14 days unless chosen")
	assert.Equal(t, "Good.", *in.Picks[0].Note)
	assert.Nil(t, in.Picks[1].Note, "blank is no note")
	assert.Nil(t, in.Message)

	_, _, errs = NormalizeInput(Input{Picks: []Pick{{ApplicationID: "a"}, {ApplicationID: "a"}}, ExpiresInDays: ptr(91)})
	assert.Len(t, errs, 2, "twice, and too long")
	_, _, errs = NormalizeInput(Input{})
	assert.Len(t, errs, 1, "nobody")
	many := make([]Pick, MaxCandidates+1)
	for i := range many {
		many[i].ApplicationID = string(rune('a' + i))
	}
	_, _, errs = NormalizeInput(Input{Picks: many, ExpiresInDays: ptr(0), Message: ptr(strings.Repeat("x", 2001))})
	assert.Len(t, errs, 3)
}

func TestStatus(t *testing.T) {
	now := time.Now()
	assert.Equal(t, Open, Status(now.Add(time.Hour), nil, now))
	assert.Equal(t, Expired, Status(now, nil, now), "expires at the moment")
	assert.Equal(t, Revoked, Status(now.Add(time.Hour), &now, now))
}

func TestNormalizeAnswer(t *testing.T) {
	c, errs := NormalizeAnswer(Approved, ptr("  Let's meet.  "))
	assert.Empty(t, errs)
	assert.Equal(t, "Let's meet.", *c)
	_, errs = NormalizeAnswer("maybe", ptr(strings.Repeat("x", 2001)))
	assert.Len(t, errs, 2)
}

func TestLabelAndFileName(t *testing.T) {
	blind := ClientCV{Blind: true, Reference: "Candidate C-4F2K9Q"}
	assert.Equal(t, "Candidate C-4F2K9Q", Label(blind, "Siti Tan"), "a blind CV never shows the name")
	assert.Equal(t, "Siti Tan", Label(ClientCV{Reference: "Candidate C-4F2K9Q"}, " Siti Tan "))
	assert.Equal(t, "Candidate C-4F2K9Q CV.pdf", CVFileName(blind.Reference, "Agency CV - Siti Tan.pdf"), "never the stored name")
	assert.Equal(t, "A-B CV.docx", CVFileName("A/B", "x.DOCX"))
	assert.Equal(t, "A CV.pdf", CVFileName("A", "x.exe"))
}

func TestTokens(t *testing.T) {
	a, ha, err := NewToken()
	require.NoError(t, err)
	b, _, err := NewToken()
	require.NoError(t, err)
	assert.Len(t, a, 43)
	assert.NotEqual(t, a, b)
	assert.Equal(t, ha, HashToken(a))
	assert.NotContains(t, ha, a, "only the hash is kept")
}

func TestLinkEmail(t *testing.T) {
	expires := time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC)
	subject, body := LinkEmail("Aisyah Rahman", "Finance Manager", "https://x/shortlist/t", 2, ptr("Two strong ones."), expires, "Dewi", "Hyppies")
	assert.Equal(t, "Shortlist for Finance Manager: 2 candidates", subject)
	assert.True(t, strings.HasPrefix(body, "Hello Aisyah,\n\nTwo strong ones.\n\n"))
	assert.Contains(t, body, "https://x/shortlist/t")
	assert.Contains(t, body, "until 6 December 2026")
	assert.True(t, strings.HasSuffix(body, "Dewi\nHyppies"))
	subject, body = LinkEmail("", "Analyst", "u", 1, nil, expires, "Dewi", "")
	assert.Equal(t, "Shortlist for Analyst: 1 candidate", subject)
	assert.True(t, strings.HasPrefix(body, "Hello,\n\nHere is"))
}
