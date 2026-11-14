package aiproviders

import (
	"testing"
	"time"
)

func TestIsDown(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	at := func(minutesAgo int) *time.Time { v := now.Add(-time.Duration(minutesAgo) * time.Minute); return &v }
	for _, tc := range []struct {
		name         string
		ok, failedAt *time.Time
		want         bool
	}{
		{"never tried", nil, nil, false},
		{"only answers", at(5), nil, false},
		{"failed and nothing since", nil, at(5), true},
		{"failed after the last answer", at(30), at(5), true},
		{"answered after the failure", at(2), at(5), false},
		{"failed long ago, nothing tried since: nobody knows", at(300), at(90), false},
	} {
		if got := IsDown(tc.ok, tc.failedAt, now); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if ProviderLabel("deepseek") != "AI 1" || ProviderLabel("claude") != "AI 2" || ProviderLabel("openai") != "AI 3" || ProviderLabel("server") != "The server's AI" {
		t.Error("the notice numbers the providers instead of naming them")
	}
}
