package entra_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
)

const (
	org      = "11111111-2222-3333-4444-555555555555"
	otherOrg = "99999999-8888-7777-6666-555555555555"
	oid      = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	client   = "client-123"
	nonce    = "nonce-abc"
)

func TestAuthorizeURLCarriesPKCEStateAndNonce(t *testing.T) {
	c := entra.New("https://login.example.test", nil)
	raw, err := c.AuthorizeURL(entra.Authorize{Authority: org, ClientID: client, RedirectURI: "https://app.test/v1/auth/microsoft/callback",
		State: "st", Nonce: nonce, Challenge: "ch", LoginHint: "sam@contoso.test"})
	require.NoError(t, err)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "/"+org+"/oauth2/v2.0/authorize", u.Path)
	q := u.Query()
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, "ch", q.Get("code_challenge"))
	assert.Equal(t, "st", q.Get("state"))
	assert.Equal(t, nonce, q.Get("nonce"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "openid profile email", q.Get("scope"))
	assert.Equal(t, "sam@contoso.test", q.Get("login_hint"))

	for _, bad := range []string{"common", "consumers", entra.ConsumersTenant, "../x", ""} {
		_, err := c.AuthorizeURL(entra.Authorize{Authority: bad})
		assert.Error(t, err, bad)
	}
}

func TestIDTokenNeverPassesOnMicrosoftsErrors(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	_, err := c.IDToken(context.Background(), entra.Exchange{Authority: org, ClientID: client, Code: "unknown"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "AADSTS")
	assert.NotContains(t, err.Error(), "script")
}

func validate(t *testing.T, c *entra.Client, raw string, want entra.Expect) (entra.Identity, error) {
	t.Helper()
	return c.Validate(context.Background(), raw, want)
}

func TestValidTokensGiveTheAccount(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	claims := entratest.Claims(org, oid, "Sam@Contoso.test", client, nonce)
	claims["preferred_username"] = "sam.upn@contoso.test"
	_, err := c.IDToken(context.Background(), entra.Exchange{Authority: org, ClientID: client, Code: "c"})
	require.Error(t, err) // no grant yet
	ms.Grant("c", entratest.Grant{Claims: claims})
	raw, err := c.IDToken(context.Background(), entra.Exchange{Authority: org, ClientID: client, Code: "c"})
	require.NoError(t, err)
	id, err := validate(t, c, raw, entra.Expect{ClientID: client, Nonce: nonce, Organisation: org})
	require.NoError(t, err)
	assert.Equal(t, entra.Identity{TenantID: org, ObjectID: oid, Email: "sam@contoso.test", Username: "sam.upn@contoso.test", Name: "Test Person"}, id)
	assert.Equal(t, []string{"sam@contoso.test", "sam.upn@contoso.test"}, id.Emails())

	_, err = validate(t, c, raw, entra.Expect{ClientID: client, Nonce: nonce}) // any organisation
	require.NoError(t, err)
	assert.Equal(t, 1, ms.KeyFetches(), "the keys are kept")
}

func TestBadTokensAreRefused(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	signed := func(change func(jwt.MapClaims)) string {
		claims := entratest.Claims(org, oid, "sam@contoso.test", client, nonce)
		change(claims)
		ms.Grant("x", entratest.Grant{Claims: claims})
		raw, err := c.IDToken(context.Background(), entra.Exchange{Authority: org, ClientID: client, Code: "x"})
		require.NoError(t, err)
		return raw
	}
	stranger := entratest.NewKey(t, "test-key-1") // the published key's ID, another key
	want := entra.Expect{ClientID: client, Nonce: nonce, Organisation: org}
	hour := time.Hour
	cases := map[string]struct {
		raw  string
		want error
	}{
		"another organisation": {signed(func(c jwt.MapClaims) { c["tid"], c["iss"] = otherOrg, entra.Issuer(otherOrg) }), entra.ErrOtherOrganisation},
		"a personal account": {signed(func(c jwt.MapClaims) {
			c["tid"], c["iss"] = entra.ConsumersTenant, entra.Issuer(entra.ConsumersTenant)
		}), entra.ErrPersonalAccount},
		"wrong audience":            {signed(func(c jwt.MapClaims) { c["aud"] = "someone-else" }), entra.ErrInvalid},
		"two audiences":             {signed(func(c jwt.MapClaims) { c["aud"] = []string{client, "someone-else"} }), entra.ErrInvalid},
		"issuer of another org":     {signed(func(c jwt.MapClaims) { c["iss"] = entra.Issuer(otherOrg) }), entra.ErrInvalid},
		"issuer of another service": {signed(func(c jwt.MapClaims) { c["iss"] = "https://sts.windows.net/" + org + "/" }), entra.ErrInvalid},
		"expired":                   {signed(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-10 * time.Minute).Unix() }), entra.ErrInvalid},
		"no expiry":                 {signed(func(c jwt.MapClaims) { delete(c, "exp") }), entra.ErrInvalid},
		"not valid yet":             {signed(func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(hour).Unix() }), entra.ErrInvalid},
		"issued in the future":      {signed(func(c jwt.MapClaims) { c["iat"] = time.Now().Add(hour).Unix() }), entra.ErrInvalid},
		"wrong nonce":               {signed(func(c jwt.MapClaims) { c["nonce"] = "another" }), entra.ErrInvalid},
		"no nonce":                  {signed(func(c jwt.MapClaims) { delete(c, "nonce") }), entra.ErrInvalid},
		"no object ID":              {signed(func(c jwt.MapClaims) { delete(c, "oid") }), entra.ErrInvalid},
		"bad signature":             {entratest.Sign(t, stranger, entratest.Claims(org, oid, "sam@contoso.test", client, nonce)), entra.ErrInvalid},
		"not RS256":                 {hs256(t), entra.ErrInvalid},
		"garbage":                   {"a.b.c", entra.ErrInvalid},
	}
	for name, tc := range cases {
		_, err := validate(t, c, tc.raw, want)
		assert.True(t, errors.Is(err, tc.want), "%s: %v", name, err)
	}
	_, err := validate(t, c, signed(func(jwt.MapClaims) {}), entra.Expect{ClientID: client, Nonce: ""})
	assert.ErrorIs(t, err, entra.ErrInvalid, "a nonce is always required")
}

func hs256(t *testing.T) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, entratest.Claims(org, oid, "sam@contoso.test", client, nonce))
	tok.Header["kid"] = "test-key-1"
	raw, err := tok.SignedString([]byte("a-shared-secret-anyone-could-guess"))
	require.NoError(t, err)
	return raw
}

func TestAnUnknownKeyFetchesTheKeysAgainOnce(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	now := time.Now()
	c.UseClock(func() time.Time { return now })
	want := entra.Expect{ClientID: client, Nonce: nonce, Organisation: org}
	claims := func() jwt.MapClaims {
		cl := entratest.Claims(org, oid, "sam@contoso.test", client, nonce)
		cl["iat"], cl["nbf"], cl["exp"] = now.Unix(), now.Unix(), now.Add(time.Hour).Unix()
		return cl
	}
	_, err := validate(t, c, entratest.Sign(t, ms.Rotate("k1"), claims()), want)
	require.NoError(t, err)
	require.Equal(t, 1, ms.KeyFetches())

	rotated := ms.Rotate("k2") // Microsoft publishes a new key
	_, err = validate(t, c, entratest.Sign(t, rotated, claims()), want)
	require.NoError(t, err, "a key not seen yet fetches the keys again")
	assert.Equal(t, 2, ms.KeyFetches())

	unpublished := entratest.NewKey(t, "never-published")
	_, err = validate(t, c, entratest.Sign(t, unpublished, claims()), want)
	assert.ErrorIs(t, err, entra.ErrInvalid)
	assert.Equal(t, 3, ms.KeyFetches())
	_, err = validate(t, c, entratest.Sign(t, unpublished, claims()), want)
	assert.ErrorIs(t, err, entra.ErrInvalid)
	assert.Equal(t, 3, ms.KeyFetches(), "a key still unknown fetches again at most once a minute")
	now = now.Add(2 * time.Minute)
	_, err = validate(t, c, entratest.Sign(t, unpublished, claims()), want)
	assert.ErrorIs(t, err, entra.ErrInvalid)
	assert.Equal(t, 4, ms.KeyFetches())

	now = now.Add(25 * time.Hour)
	_, err = validate(t, c, entratest.Sign(t, rotated, claims()), want)
	require.NoError(t, err)
	assert.Equal(t, 5, ms.KeyFetches(), "keys are fetched again after a day")
}

func TestIDsAndAddresses(t *testing.T) {
	assert.True(t, entra.IsOrganisation(org))
	assert.False(t, entra.IsOrganisation(entra.ConsumersTenant))
	assert.False(t, entra.IsOrganisation(strings.ToUpper(oid)), "normal form only")
	assert.Equal(t, oid, entra.NormalizeID(" "+strings.ToUpper(oid)+" "))
	assert.Equal(t, "https://login.microsoftonline.com/"+org+"/v2.0", entra.Issuer(org))
	assert.Empty(t, entra.Identity{Email: "", Username: "not-an-address"}.Emails())
}
