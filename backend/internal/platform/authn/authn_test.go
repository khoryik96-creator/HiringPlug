package authn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tenantID = "tnt_01jbz6w0m0k8q9x2r3s4t5v6w7"
	userID   = "usr_01jbz6w0m0k8q9x2r3s4t5v6w7"
)

func verifier() *Verifier {
	return NewVerifier([]byte("secret-0123456789abcdefghijklmnopqrstuv"), "hiringplug", "hiringplug-api")
}

func TestMintedTokensVerify(t *testing.T) {
	token, err := verifier().Mint(tenantID, userID, time.Hour)
	require.NoError(t, err)
	claims, err := verifier().Verify(token)
	require.NoError(t, err)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, userID, claims.Subject)
}

func TestBadTokensAreRejected(t *testing.T) {
	expired, _ := verifier().Mint(tenantID, userID, -time.Minute)
	otherAudience, _ := NewVerifier([]byte("secret-0123456789abcdefghijklmnopqrstuv"), "hiringplug", "elsewhere").Mint(tenantID, userID, time.Hour)
	otherKey, _ := NewVerifier([]byte("another-secret-0123456789abcdefghijklm"), "hiringplug", "hiringplug-api").Mint(tenantID, userID, time.Hour)
	badIDs, _ := verifier().Mint("acme", "bob", time.Hour)
	for name, token := range map[string]string{
		"expired": expired, "wrong audience": otherAudience, "wrong key": otherKey, "malformed IDs": badIDs, "garbage": "x.y.z",
	} {
		_, err := verifier().Verify(token)
		assert.ErrorIs(t, err, ErrInvalidToken, name)
	}
}
