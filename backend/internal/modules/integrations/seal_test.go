package integrations

import (
	"strings"
	"testing"
)

func TestWebhookSecretsAreSealedWithTheirOwnKey(t *testing.T) {
	signIn := []byte("sign-in-secret-0123456789-0123456789")
	s := NewSealer([]byte("webhook-keys-secret-0123456789-0123456789"), signIn)
	stored, err := s.Seal("tnt_a", "whsec_abc")
	if err != nil || strings.Contains(stored, "whsec_abc") || !strings.HasPrefix(stored, "v1.") {
		t.Fatalf("stored %q, err %v: the secret must not be readable in what is stored", stored, err)
	}
	if got, restore, err := s.Open("tnt_a", stored); err != nil || got != "whsec_abc" || restore {
		t.Fatalf("open: %q %v %v", got, restore, err)
	}
	if _, _, err := s.Open("tnt_b", stored); err == nil {
		t.Error("another company's webhook must not open it")
	}

	// A new sign-in secret (moving to production) changes nothing for webhooks.
	moved := NewSealer([]byte("webhook-keys-secret-0123456789-0123456789"), []byte("a-new-sign-in-secret-0123456789-012345"))
	if got, _, err := moved.Open("tnt_a", stored); err != nil || got != "whsec_abc" {
		t.Errorf("a new sign-in secret must not break webhooks: %q %v", got, err)
	}

	// What an earlier version sealed with the sign-in secret still opens, and is sealed again.
	earlier := Sealer{key: derive(signIn)}
	old, _ := earlier.Seal("tnt_a", "whsec_old")
	if got, restore, err := s.Open("tnt_a", old); err != nil || got != "whsec_old" || !restore {
		t.Errorf("an earlier version's seal: %q %v %v", got, restore, err)
	}

	// Saved before sealing: still works, sealed the next time it is used.
	if got, restore, err := s.Open("tnt_a", "whsec_plain"); err != nil || got != "whsec_plain" || !restore {
		t.Errorf("one saved in the clear: %q %v %v", got, restore, err)
	}

	// Without HP_WEBHOOK_KEYS_SECRET: kept as they are.
	none := NewSealer(nil, signIn)
	if got, _ := none.Seal("tnt_a", "whsec_x"); got != "whsec_x" {
		t.Errorf("no key: kept as it is, got %q", got)
	}
	if got, restore, err := none.Open("tnt_a", "whsec_x"); err != nil || got != "whsec_x" || restore {
		t.Errorf("no key: %q %v %v", got, restore, err)
	}
	if _, _, err := NewSealer([]byte("another-webhook-keys-secret-0123456789"), nil).Open("tnt_a", stored); err == nil {
		t.Error("another key must not open it")
	}
}
