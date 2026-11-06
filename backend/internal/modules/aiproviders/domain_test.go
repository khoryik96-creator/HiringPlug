package aiproviders

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

// The AI worker's tests open this very value (tests/test_ai_keys.py), so the
// two languages cannot drift apart.
const (
	vectorSecret = "test-secret-0123456789-0123456789-abcdef"
	vectorKey    = "sk-test-0123456789abcdef"
	vectorSealed = "v1.AAECAwQFBgcICQoL0L_sahf3mXCUsEjZQevTMjksVWQCL7wDjTIRxzMizn_bZoPTlfISJw"
)

func TestSealMatchesTheWorkersVector(t *testing.T) {
	got, err := Seal(bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}), []byte(vectorSecret), "deepseek", vectorKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != vectorSealed {
		t.Fatalf("Seal = %q, want %q", got, vectorSealed)
	}
}

func TestSealUsesAFreshNonceEachTime(t *testing.T) {
	a, _ := Seal(rand.Reader, []byte(vectorSecret), "claude", vectorKey)
	b, _ := Seal(rand.Reader, []byte(vectorSecret), "claude", vectorKey)
	if a == b || !strings.HasPrefix(a, "v1.") || strings.Contains(a, vectorKey) {
		t.Fatalf("sealed keys must differ, start with v1. and hide the key: %q %q", a, b)
	}
}

func TestCheckInput(t *testing.T) {
	key := "  sk-abcdef123456  "
	model, k, err := CheckInput(" deepseek-v4-flash ", &key)
	if err != nil || model != "deepseek-v4-flash" || *k != "sk-abcdef123456" {
		t.Fatalf("got %q %q %v", model, *k, err)
	}
	for _, bad := range []string{"short", "sk-has space-inside", "sk-ключ-123456"} {
		if _, _, err := CheckInput("gpt-5.5", &bad); err == nil {
			t.Errorf("key %q was accepted", bad)
		}
	}
	if _, _, err := CheckInput("bad model!", nil); err == nil {
		t.Error("a bad model name was accepted")
	}
	if _, _, err := CheckInput("claude-opus-5-5", nil); err != nil {
		t.Errorf("saving a model alone (keeping the key) failed: %v", err)
	}
}

func TestHint(t *testing.T) {
	if got := Hint("sk-abcdef1234"); got != "…1234" {
		t.Errorf("Hint = %q", got)
	}
	if got := Hint("abc"); got != "…" {
		t.Errorf("a short key showed %q", got)
	}
}

func TestPick(t *testing.T) {
	entries := []Entry{{Provider: "claude", Model: "claude-opus-5-5", SealedKey: "v1.x"}}
	cases := []struct {
		choice   string
		serverOn bool
		want     string
		on       bool
	}{
		{"", true, "", true},       // no choice yet: the server's own
		{"", false, "", false},     // ... which is off
		{"server", true, "", true}, //
		{"off", true, "", false},   // switched off although the server has one
		{"claude", false, "claude", true},
		{"openai", true, "", false}, // chosen but no key: off, not the server's
	}
	for _, c := range cases {
		e, on := Pick(c.choice, c.serverOn, entries)
		got := ""
		if e != nil {
			got = e.Provider
		}
		if got != c.want || on != c.on {
			t.Errorf("Pick(%q, %v) = %q %v, want %q %v", c.choice, c.serverOn, got, on, c.want, c.on)
		}
	}
}
