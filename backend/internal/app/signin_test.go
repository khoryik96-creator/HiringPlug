package app_test

import (
	"context"
	"io"
	"mime/quotedprintable"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

const testPassword = "purple-gecko-41"

func setPassword(t *testing.T, userID, password string) {
	t.Helper()
	require.NoError(t, app.SetPassword(context.Background(), testutil.Current.Admin, userID, password))
}

func emailOf(role api.Role, tenantID string) string {
	return string(role) + "." + tenantID + "@test.local"
}

func (a *testApp) signIn(body map[string]any) (*response, string) {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/auth/sign-in", "", body)
	var s api.Session
	if res.Status == http.StatusOK {
		res.JSON(&s)
	}
	return res, s.Token
}

var linkRe = regexp.MustCompile(`/#/(invite|reset)/([0-9a-f]{64})`)

// linkIn returns the link token in the last email to address.
func linkIn(t *testing.T, address string) string {
	t.Helper()
	msgs := mailServer.To(address)
	require.NotEmpty(t, msgs, "an email to %s", address)
	data := msgs[len(msgs)-1].Data
	_, body, _ := strings.Cut(data, "\r\n\r\n")
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	require.NoError(t, err)
	m := linkRe.FindStringSubmatch(string(decoded))
	require.NotNil(t, m, "a link in: %s", decoded)
	return m[2]
}

func TestSigningIn(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner]
	email := emailOf(api.RoleOwner, tenant.ID)
	setPassword(t, owner.ID, testPassword)

	res, token := a.signIn(map[string]any{"email": strings.ToUpper(email), "password": testPassword})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", token, nil).Status)

	res, _ = a.signIn(map[string]any{"email": email, "password": "wrong-password-1"})
	assert.Equal(t, api.ErrorCodeSIGNINFAILED, res.Error().Error.Code)
	res, _ = a.signIn(map[string]any{"email": "nobody." + tenant.ID + "@test.local", "password": testPassword})
	assert.Equal(t, api.ErrorCodeSIGNINFAILED, res.Error().Error.Code, "the same answer for an unknown email")

	t.Run("one email at two companies chooses a company", func(t *testing.T) {
		shared := "shared." + tenant.ID + "@test.local"
		ids := []string{}
		for _, name := range []string{"Alpha", "Beta"} {
			tid, err := app.CreateTenant(context.Background(), testutil.Current.Admin, name+" "+tenant.ID)
			require.NoError(t, err)
			uid, err := app.CreateUser(context.Background(), testutil.Current.Admin, tid, shared, "Sam", api.RoleRecruiter)
			require.NoError(t, err)
			setPassword(t, uid, testPassword)
			ids = append(ids, uid)
		}
		res, _ := a.signIn(map[string]any{"email": shared, "password": testPassword})
		e := res.Error()
		require.Equal(t, api.ErrorCodeSIGNINCHOOSECOMPANY, e.Error.Code)
		companies := (*e.Error.Details)["companies"].([]any)
		require.Len(t, companies, 2)
		slug := companies[1].(map[string]any)["slug"].(string)
		res, token := a.signIn(map[string]any{"email": shared, "password": testPassword, "company": slug})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		me := a.request(http.MethodGet, "/v1/me", token, nil).Map()
		assert.Equal(t, ids[1], me["user"].(map[string]any)["id"])
	})

	t.Run("guessing is cut off", func(t *testing.T) {
		victim := "victim." + tenant.ID + "@test.local"
		var last int
		for range 11 {
			res, _ := a.signIn(map[string]any{"email": victim, "password": "guess-guess-1"})
			last = res.Status
		}
		assert.Equal(t, http.StatusTooManyRequests, last)
	})

	t.Run("changing the password ends other sessions", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/me/password", token, map[string]any{"currentPassword": "nope", "newPassword": "another-gecko-42"})
		assert.Equal(t, []string{"currentPassword"}, fieldNames(t, res.Error()))
		res = a.request(http.MethodPut, "/v1/me/password", token, map[string]any{"currentPassword": testPassword, "newPassword": "another-gecko-42"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var s api.Session
		res.JSON(&s)
		assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", token, nil).Status, "the old session")
		assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/me", s.Token, nil).Status, "this browser's new one")
	})

	t.Run("a forgotten password is reset by email", func(t *testing.T) {
		require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": email}).Status)
		assert.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/auth/password-resets", "", map[string]any{"email": "nobody@test.local"}).Status, "no hint")
		runCoreJobsUntilIdle(t, a, tenant.ID)
		link := linkIn(t, email)
		info := a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).Map()
		assert.Equal(t, "reset", info["kind"])
		res := a.request(http.MethodPost, "/v1/auth/links/"+link, "", map[string]any{"password": "short"})
		assert.Equal(t, http.StatusBadRequest, res.Status)
		res = a.request(http.MethodPost, "/v1/auth/links/"+link, "", map[string]any{"password": "third-gecko-43"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, http.StatusGone, a.request(http.MethodPost, "/v1/auth/links/"+link, "", map[string]any{"password": "fourth-gecko-44"}).Status, "used once")
		res, _ = a.signIn(map[string]any{"email": email, "password": "third-gecko-43"})
		assert.Equal(t, http.StatusOK, res.Status)
	})
}

func TestTeamAdmin(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	newEmail := "new." + tenant.ID + "@test.local"

	res := a.request(http.MethodPost, "/v1/team/invites", admin.Token, map[string]any{"email": newEmail, "name": "New Person", "role": "sourcer"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var inv api.Invitation
	res.JSON(&inv)
	assert.Equal(t, api.TeamStatusInvited, inv.Member.Status)
	assert.Contains(t, inv.Link, "/#/invite/", "the link, to pass on if email is not set up")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/team/invites", recruiter.Token, map[string]any{"email": "x@test.local", "name": "X", "role": "viewer"}).Status)
	res = a.request(http.MethodPost, "/v1/team/invites", admin.Token, map[string]any{"email": strings.ToUpper(newEmail), "name": "Again", "role": "viewer"})
	assert.Equal(t, []string{"email"}, fieldNames(t, res.Error()))
	res = a.request(http.MethodPost, "/v1/team/invites", admin.Token, map[string]any{"email": "boss." + tenant.ID + "@test.local", "name": "Boss", "role": "owner"})
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "only owners make owners")

	t.Run("the invited person joins with the emailed link", func(t *testing.T) {
		runCoreJobsUntilIdle(t, a, tenant.ID)
		first := linkIn(t, newEmail)
		res := a.request(http.MethodPost, "/v1/team/"+inv.Member.Id+"/invite", admin.Token, nil)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		assert.Equal(t, http.StatusGone, a.request(http.MethodGet, "/v1/auth/links/"+first, "", nil).Status, "replaced by the new invitation")
		runCoreJobsUntilIdle(t, a, tenant.ID)
		link := linkIn(t, newEmail)
		info := a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).Map()
		assert.Equal(t, "invite", info["kind"])
		assert.Equal(t, "New Person", info["name"])
		res = a.request(http.MethodPost, "/v1/auth/links/"+link, "", map[string]any{"password": testPassword, "name": " Nur  Aisyah "})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var s api.Session
		res.JSON(&s)
		me := a.request(http.MethodGet, "/v1/me", s.Token, nil).Map()
		assert.Equal(t, "Nur Aisyah", me["user"].(map[string]any)["name"])
		assert.Equal(t, "sourcer", me["role"])
	})

	t.Run("the team rules", func(t *testing.T) {
		patch := func(token, id string, body map[string]any) *response {
			return a.request(http.MethodPatch, "/v1/team/"+id, token, body)
		}
		assert.Equal(t, http.StatusUnprocessableEntity, patch(admin.Token, admin.ID, map[string]any{"active": false}).Status, "not yourself")
		assert.Equal(t, http.StatusUnprocessableEntity, patch(admin.Token, owner.ID, map[string]any{"role": "admin"}).Status, "owners by owners")
		assert.Equal(t, http.StatusUnprocessableEntity, patch(owner.Token, owner.ID, map[string]any{"role": "admin"}).Status, "not your own role")
		res := patch(admin.Token, recruiter.ID, map[string]any{"active": false})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", recruiter.Token, nil).Status, "deactivated: signed out")
		require.Equal(t, http.StatusOK, patch(admin.Token, recruiter.ID, map[string]any{"active": true, "role": "sourcer"}).Status)
		assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", recruiter.Token, nil).Status, "old sessions stay ended")

		var team api.TeamList
		a.request(http.MethodGet, "/v1/team", owner.Token, nil).JSON(&team)
		assert.Len(t, team.Items, 4)
	})
}
