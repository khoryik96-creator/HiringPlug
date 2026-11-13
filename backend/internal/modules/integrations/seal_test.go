package integrations

import (
	"strings"
	"testing"
)

func TestWebhookSecretsAreSealed(t *testing.T) {
	s := NewSealer([]byte("server-secret-0123456789-0123456789"))
	stored, err := s.Seal("tnt_a", "whsec_abc")
	if err != nil || strings.Contains(stored, "whsec_abc") || !strings.HasPrefix(stored, "v1.") {
		t.Fatalf("stored %q, err %v: the secret must not be readable in what is stored", stored, err)
	}
	if got, sealed, err := s.Open("tnt_a", stored); err != nil || got != "whsec_abc" || !sealed {
		t.Fatalf("open: %q %v %v", got, sealed, err)
	}
	if _, _, err := s.Open("tnt_b", stored); err == nil {
		t.Error("another company's webhook must not open it")
	}
	if _, _, err := NewSealer([]byte("another-server-secret-0123456789")).Open("tnt_a", stored); err == nil {
		t.Error("another server secret must not open it")
	}
	if got, sealed, err := s.Open("tnt_a", "whsec_old"); err != nil || got != "whsec_old" || sealed {
		t.Errorf("one saved before sealing still works: %q %v %v", got, sealed, err)
	}
}
