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
	assert.Equal(t, entra.Identity{TenantID: org, ObjectID: oid, Email: "sam@contoso.test", Username: "sam.upn@contoso.test", Member: true, Name: "Test Person"}, id)
	assert.Equal(t, []string{"sam.upn@contoso.test"}, id.Addresses(), "the email is not vouched for: the sign-in name only")

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
	assert.Empty(t, entra.Identity{Email: "", Username: "not-an-address"}.Addresses())
}

func TestTheEmailCountsOnlyWhenMicrosoftVouchesForIt(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	want := entra.Expect{ClientID: client, Nonce: nonce, Organisation: org}
	for _, tc := range []struct {
		name  string
		edov  any // the xms_edov claim; nil: not sent
		addrs []string
	}{
		{"not sent", nil, []string{"sam.upn@contoso.test"}},
		{"false", false, []string{"sam.upn@contoso.test"}},
		{"true", true, []string{"sam.upn@contoso.test", "sam@contoso.test"}},
		{"\"1\"", "1", []string{"sam.upn@contoso.test", "sam@contoso.test"}},
		{"1", 1, []string{"sam.upn@contoso.test", "sam@contoso.test"}},
	} {
		claims := entratest.Claims(org, oid, "Sam@Contoso.test", client, nonce)
		claims["preferred_username"] = "sam.upn@contoso.test"
		if tc.edov != nil {
			claims["xms_edov"] = tc.edov
		}
		id, err := validate(t, c, entratest.Sign(t, ms.SigningKey(), claims), want)
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.addrs, id.Addresses(), tc.name)
	}
	same := entra.Identity{Email: "sam@contoso.test", Username: "sam@contoso.test", EmailVerified: true, Member: true}
	assert.Equal(t, []string{"sam@contoso.test"}, same.Addresses(), "each once")
}

// Without the acct claim a guest cannot be told from a member (its
// preferred_username is its home address): no address counts then.
func TestNoAddressCountsWithoutTheAcctClaim(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	want := entra.Expect{ClientID: client, Nonce: nonce, Organisation: org}
	claims := entratest.VerifiedEmail(entratest.WithoutAcct(entratest.Claims(org, oid, "sam@contoso.test", client, nonce)), true)
	id, err := validate(t, c, entratest.Sign(t, ms.SigningKey(), claims), want)
	require.NoError(t, err, "the account itself is valid")
	assert.False(t, id.Member)
	assert.Empty(t, id.Addresses())
	assert.Equal(t, "sam@contoso.test", id.Username, "still said (for the email telling them)")
}

func TestGuestsAreRefused(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	want := entra.Expect{ClientID: client, Nonce: nonce, Organisation: org}
	sign := func(claims jwt.MapClaims) string { return entratest.Sign(t, ms.SigningKey(), claims) }
	member := entratest.Claims(org, oid, "sam@contoso.test", client, nonce)
	_, err := validate(t, c, sign(member), want)
	require.NoError(t, err, "acct 0: a member")

	cases := map[string]jwt.MapClaims{
		"acct 1":         entratest.Guest(entratest.Claims(org, oid, "sam@fabrikam.test", client, nonce), true, false),
		"acct 1 and upn": entratest.Guest(entratest.Claims(org, oid, "sam@fabrikam.test", client, nonce), true, true),
		"acct \"1\"": func() jwt.MapClaims {
			cl := entratest.Claims(org, oid, "sam@contoso.test", client, nonce)
			cl["acct"] = "1"
			return cl
		}(),
		"a guest's UPN, no acct": entratest.Guest(entratest.Claims(org, oid, "sam@fabrikam.test", client, nonce), false, true),
		"a guest's UPN as sign-in name": func() jwt.MapClaims {
			cl := entratest.Claims(org, oid, "sam@contoso.test", client, nonce)
			cl["preferred_username"] = entratest.GuestUPN("sam@fabrikam.test", "contoso.onmicrosoft.com")
			return cl
		}(),
		"a guest's name as email": func() jwt.MapClaims {
			cl := entratest.Claims(org, oid, "sam@contoso.test", client, nonce)
			cl["email"] = entratest.GuestUPN("sam@contoso.test", "Fabrikam.onmicrosoft.com")
			return cl
		}(),
	}
	for name, claims := range cases {
		_, err := validate(t, c, sign(claims), want)
		assert.ErrorIs(t, err, entra.ErrGuestAccount, name)
	}
	// Neither acct nor upn sent: nothing tells it is a guest; it is accepted,
	// but recognised by no address (see TestNoAddressCountsWithoutTheAcctClaim).
	id, err := validate(t, c, sign(entratest.Guest(entratest.Claims(org, oid, "sam@fabrikam.test", client, nonce), false, false)), want)
	require.NoError(t, err)
	assert.Equal(t, "sam@fabrikam.test", id.Username, "a guest's preferred_username is its home address")
	assert.Empty(t, id.Addresses())
	assert.Empty(t, entra.Identity{Username: entratest.GuestUPN("sam@contoso.test", "f.onmicrosoft.com")}.Addresses())
}

func TestMadeUpKeyIDsFetchTheKeysOnlyAFewTimes(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	want := entra.Expect{ClientID: client, Nonce: nonce, Organisation: org}
	stranger := entratest.NewKey(t, "")
	for i := range 20 {
		made := entratest.Key{ID: "made-up-" + string(rune('a'+i)), Private: stranger.Private} // never published
		_, err := validate(t, c, entratest.Sign(t, made, entratest.Claims(org, oid, "sam@contoso.test", client, nonce)), want)
		assert.ErrorIs(t, err, entra.ErrInvalid)
	}
	assert.LessOrEqual(t, ms.KeyFetches(), 6, "a few fetches, not one per made-up key ID")
	_, err := validate(t, c, entratest.Sign(t, ms.SigningKey(), entratest.Claims(org, oid, "sam@contoso.test", client, nonce)), want)
	require.NoError(t, err, "the published key still works")
}

func TestFetchingOneOrganisationsKeysKeepsNoOtherWaiting(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	release := ms.HoldKeys(org)
	defer release()
	first := entratest.Sign(t, ms.SigningKey(), entratest.Claims(org, oid, "sam@contoso.test", client, nonce))
	second := entratest.Sign(t, ms.SigningKey(), entratest.Claims(otherOrg, oid, "sam@contoso.test", client, nonce))
	slow := make(chan error, 1)
	go func() {
		_, err := c.Validate(context.Background(), first, entra.Expect{ClientID: client, Nonce: nonce, Organisation: org})
		slow <- err
	}()
	for ms.KeyFetchesWaiting() == 0 { // the first organisation's fetch is under way
		time.Sleep(time.Millisecond)
	}
	other := make(chan error, 1)
	go func() {
		_, err := c.Validate(context.Background(), second, entra.Expect{ClientID: client, Nonce: nonce, Organisation: otherOrg})
		other <- err
	}()
	select {
	case err := <-other:
		require.NoError(t, err, "another organisation's keys")
	case <-time.After(5 * time.Second):
		t.Fatal("another organisation waited for the first one's keys")
	}
	release()
	require.NoError(t, <-slow)
}

func TestManyAtOnceFetchAnOrganisationsKeysOnce(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	release := ms.HoldKeys(org)
	raw := entratest.Sign(t, ms.SigningKey(), entratest.Claims(org, oid, "sam@contoso.test", client, nonce))
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := c.Validate(context.Background(), raw, entra.Expect{ClientID: client, Nonce: nonce, Organisation: org})
			errs <- err
		}()
	}
	for ms.KeyFetchesWaiting() == 0 {
		time.Sleep(time.Millisecond)
	}
	// Released at once: whoever has not asked yet finds the keys kept, or
	// fetched by the first meanwhile; never a second fetch.
	release()
	for range 8 {
		require.NoError(t, <-errs)
	}
	assert.Equal(t, 1, ms.KeyFetches())
}

// A request that ends (the browser went away) stops waiting for keys
// another request is fetching.
func TestWaitingForKeysEndsWithTheRequest(t *testing.T) {
	ms := entratest.New(t)
	c := entra.New(ms.URL, ms.Client())
	release := ms.HoldKeys(org)
	defer release()
	raw := entratest.Sign(t, ms.SigningKey(), entratest.Claims(org, oid, "sam@contoso.test", client, nonce))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Validate(ctx, raw, entra.Expect{ClientID: client, Nonce: nonce, Organisation: org})
		done <- err
	}()
	for ms.KeyFetchesWaiting() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
		assert.ErrorIs(t, err, entra.ErrInvalid)
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting for the keys after the request ended")
	}
	release()
	_, err := c.Validate(context.Background(), raw, entra.Expect{ClientID: client, Nonce: nonce, Organisation: org})
	require.NoError(t, err, "the fetch went on for whoever asks next")
	assert.Equal(t, 1, ms.KeyFetches())
}
