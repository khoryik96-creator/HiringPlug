package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestCompanyCategories(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token

	t.Run("the categories are the agency's, each with a colour", func(t *testing.T) {
		var all api.FieldSettings
		a.request(http.MethodGet, "/v1/settings/fields", recruiter, nil).JSON(&all)
		var cats api.FieldList
		for _, l := range all.Lists {
			if l.Name == api.FieldListNameCompanyCategory {
				cats = l
			}
		}
		require.True(t, cats.Colors)
		require.Len(t, cats.Options, 4)
		assert.Equal(t, "VIP client", cats.Options[3].Label)
		assert.Equal(t, "#b8860b", cats.Options[3].Color.MustGet())

		options := []map[string]any{}
		for _, o := range cats.Options {
			options = append(options, map[string]any{"key": o.Key, "label": o.Label, "color": o.Color.MustGet()})
		}
		options[3]["label"], options[3]["color"] = "Key account (VIP)", "#7C3AED"
		options = append(options, map[string]any{"label": "Dormant", "color": "#94a3b8"})
		res := a.request(http.MethodPut, "/v1/settings/fields/lists/company_category", owner, map[string]any{"options": options})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var saved api.FieldList
		res.JSON(&saved)
		assert.Equal(t, "Key account (VIP)", saved.Options[3].Label)
		assert.Equal(t, "#7c3aed", saved.Options[3].Color.MustGet())
		assert.Equal(t, "dormant", saved.Options[4].Key)
	})

	vip := a.createCompany(recruiter, map[string]any{"name": "Petronas Dagangan", "category": "vip_client"})
	assert.Equal(t, "vip_client", vip.Category.MustGet())
	prospect := a.createCompany(recruiter, map[string]any{"name": "Grab Malaysia", "type": "prospect"})
	assert.True(t, prospect.Category.IsNull(), "not set: shown as Unknown")

	res := a.request(http.MethodPatch, "/v1/companies/"+prospect.Id, recruiter, map[string]any{"version": prospect.Version, "category": "signed_client"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var signed api.Company
	res.JSON(&signed)
	assert.Equal(t, "signed_client", signed.Category.MustGet())
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPatch, "/v1/companies/"+prospect.Id, recruiter,
		map[string]any{"version": signed.Version, "category": "Not A Key"}).Status)

	var page api.CompanyPage
	a.request(http.MethodGet, "/v1/companies?category=vip_client", recruiter, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, vip.Id, page.Items[0].Id)
	assert.Equal(t, "vip_client", page.Items[0].Category.MustGet())
}
