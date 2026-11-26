package app_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestOutlookSuggestsTheAddressesInYourOwnMail(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	token, coworker := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	setupOutlookMail(t, a, token)
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	suggest := func(bearer, typed string) []api.OutlookAddress {
		r := a.request("GET", "/v1/outlook/addresses?q="+typed, bearer, nil)
		require.Equal(t, 200, r.Status, "%s", r.Body)
		var out api.OutlookAddressList
		r.JSON(&out)
		return out.Items
	}
	all := suggest(token, "")
	require.Len(t, all, 1, "Nadia wrote; the recruiter's own address is not suggested: %+v", all)
	assert.Equal(t, "nadia@example.test", all[0].Address)
	assert.Equal(t, "Nadia Rahman", all[0].Name.MustGet(), "named as her own mail names her")
	assert.Len(t, suggest(token, "NADIA@"), 1, "part of the address, in any case")
	assert.Len(t, suggest(token, "rahman"), 1, "part of the name")
	assert.Empty(t, suggest(token, "lina"))
	assert.Empty(t, suggest(coworker, ""), "no one else's mail is suggested")
	assert.Equal(t, 400, a.request("GET", "/v1/outlook/addresses?q="+strings.Repeat("a", 101), token, nil).Status)
}
