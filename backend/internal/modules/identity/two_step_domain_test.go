package identity

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RFC 6238's own test secret ("12345678901234567890") and times, 6 digits.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestCodesAreWhatAuthenticatorAppsShow(t *testing.T) {
	for at, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		got, err := TOTPCode(rfcSecret, TOTPStep(time.Unix(at, 0)))
		require.NoError(t, err)
		assert.Equal(t, want, got, "at %d", at)
	}
}

func TestACodeWorksOnceAndOnlyNearItsTime(t *testing.T) {
	now := time.Unix(1111111109, 0)
	step, ok := CheckTOTP(rfcSecret, "081 804", now, 0)
	assert.True(t, ok)
	_, ok = CheckTOTP(rfcSecret, "081804", now, step)
	assert.False(t, ok, "used already")
	_, ok = CheckTOTP(rfcSecret, "081804", now.Add(30*time.Second), 0)
	assert.True(t, ok, "a phone a step behind")
	_, ok = CheckTOTP(rfcSecret, "081804", now.Add(2*time.Minute), 0)
	assert.False(t, ok, "too late")
	_, ok = CheckTOTP(rfcSecret, "123456", now, 0)
	assert.False(t, ok)
}

func TestRecoveryCodesAreSingleSetAndKeptAsHashes(t *testing.T) {
	codes, hashes := NewRecoveryCodes()
	require.Len(t, codes, 10)
	assert.Len(t, hashes, 10)
	assert.Regexp(t, `^[a-z2-9]{5}-[a-z2-9]{5}$`, codes[0])
	assert.Equal(t, hashes[0], HashRecoveryCode(" "+strings.ToUpper(strings.ReplaceAll(codes[0], "-", ""))+" "))
	assert.NotContains(t, hashes[0], codes[0])
	assert.True(t, LooksLikeRecoveryCode(codes[0]))
	assert.False(t, LooksLikeRecoveryCode("123456"))
}

func TestChallengesAreSignedAndShortLived(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now()
	token := SignChallenge(key, Challenge{TenantID: "tnt_1", UserID: "usr_1", Expires: now.Add(challengeLife).Unix()})
	c, err := ReadChallenge(key, token, now)
	require.NoError(t, err)
	assert.Equal(t, "usr_1", c.UserID)
	_, err = ReadChallenge(key, token, now.Add(11*time.Minute))
	assert.Error(t, err, "expired")
	_, err = ReadChallenge([]byte("another key, another key, another"), token, now)
	assert.Error(t, err, "not ours")
	payload, mac, _ := strings.Cut(token, ".")
	_, err = ReadChallenge(key, payload+"x."+mac, now)
	assert.Error(t, err, "changed")
}

func TestTheAppIsToldTheCompanyAndPerson(t *testing.T) {
	uri := TOTPURI("ABC", "Hyppies Sdn Bhd", "siti@hyppies.example")
	assert.True(t, strings.HasPrefix(uri, "otpauth://totp/HiringPlug%20%28Hyppies%20Sdn%20Bhd%29:siti@hyppies.example?"), uri)
	assert.Contains(t, uri, "secret=ABC")
	assert.Contains(t, uri, "issuer=HiringPlug")
}
