package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) createContact(token string, body map[string]any) api.Contact {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/contacts", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var c api.Contact
	res.JSON(&c)
	return c
}

func TestContactsCreateWithEmailCheck(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maybank"})

	c := a.createContact(token, map[string]any{
		"companyId": company.Id, "firstName": " Aisyah ", "lastName": "Rahman", "title": "HR Manager",
		"email": " Aisyah.Rahman@Maybank.com.my ", "linkedinUrl": "linkedin.com/in/aisyah-rahman",
	})
	assert.Equal(t, "Aisyah Rahman", c.FullName)
	assert.Equal(t, "Maybank", c.CompanyName)
	email, _ := c.Email.Get()
	assert.Equal(t, "aisyah.rahman@maybank.com.my", email)
	check, err := c.EmailCheck.Get()
	require.NoError(t, err)
	assert.Equal(t, api.EmailCheckStatusValid, check.Status)
	assert.Empty(t, check.Reasons)
	linkedin, _ := c.LinkedinUrl.Get()
	assert.Equal(t, "https://www.linkedin.com/in/aisyah-rahman", linkedin)
	assert.Equal(t, api.ContactStatusActive, c.Status)

	tests := []struct{ email, status, reason string }{
		{"hr@maybank.com.my", "risky", "a shared mailbox (hr@), not a person"},
		{"temp@mailinator.com", "risky", "a disposable address that stops working soon"},
		{"wei@acme.nomail.test", "invalid", "the domain acme.nomail.test does not receive email"},
		{"wei@acme.dnsdown.test", "unknown", "could not reach the domain's mail server; check again later"},
	}
	for _, tc := range tests {
		got := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "X", "email": tc.email})
		check, err := got.EmailCheck.Get()
		require.NoError(t, err, tc.email)
		assert.Equal(t, api.EmailCheckStatus(tc.status), check.Status, tc.email)
		assert.Equal(t, []string{tc.reason}, check.Reasons, tc.email)
	}

	none := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "No email"})
	assert.True(t, none.EmailCheck.IsNull(), "no email, no check")
}

func TestContactsValidationAndDuplicates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Grab"})
	otherTenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	foreign := a.createCompany(otherTenant.Users[api.RoleRecruiter].Token, map[string]any{"name": "Grab"})

	tests := []struct {
		name   string
		body   map[string]any
		fields []string
	}{
		{"bad email", map[string]any{"companyId": company.Id, "firstName": "Wei", "email": "wei@"}, []string{"email"}},
		{"not linkedin", map[string]any{"companyId": company.Id, "firstName": "Wei", "linkedinUrl": "facebook.com/wei"}, []string{"linkedinUrl"}},
		{"blank name", map[string]any{"companyId": company.Id, "firstName": "  "}, []string{"firstName"}},
		{"another tenant's company", map[string]any{"companyId": foreign.Id, "firstName": "Wei"}, []string{"companyId"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := a.request(http.MethodPost, "/v1/contacts", token, tc.body)
			require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
			assert.ElementsMatch(t, tc.fields, fieldNames(t, res.Error()))
		})
	}

	first := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Wei", "email": "wei@grab.com"})
	res := a.request(http.MethodPost, "/v1/contacts", token, map[string]any{"companyId": company.Id, "firstName": "Wei Two", "email": "WEI@grab.com"})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeCONTACTDUPLICATEEMAIL, res.Error().Error.Code)
	assert.Equal(t, first.Id, (*res.Error().Error.Details)["existingContactId"])
}

func TestContactsUpdateAndRecheck(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	maybank := a.createCompany(token, map[string]any{"name": "Maybank"})
	cimb := a.createCompany(token, map[string]any{"name": "CIMB"})
	c := a.createContact(token, map[string]any{"companyId": maybank.Id, "firstName": "Aisyah", "email": "aisyah@x.dnsdown.test"})

	// Moving to another company with a new email: the email is checked again.
	res := a.request(http.MethodPatch, "/v1/contacts/"+c.Id, token, map[string]any{
		"version": c.Version, "companyId": cimb.Id, "email": "aisyah@cimb.com", "title": "Head of HR",
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var moved api.Contact
	res.JSON(&moved)
	assert.Equal(t, "CIMB", moved.CompanyName)
	check, _ := moved.EmailCheck.Get()
	assert.Equal(t, api.EmailCheckStatusValid, check.Status)
	assert.Equal(t, 2, moved.Version)

	// The old version is refused.
	res = a.request(http.MethodPatch, "/v1/contacts/"+c.Id, token, map[string]any{"version": c.Version, "status": "left"})
	assert.Equal(t, http.StatusConflict, res.Status)

	// Unchanged: nothing written.
	res = a.request(http.MethodPatch, "/v1/contacts/"+c.Id, token, map[string]any{"version": moved.Version, "title": "Head of HR"})
	var same api.Contact
	res.JSON(&same)
	assert.Equal(t, moved.Version, same.Version)

	// "Check again" re-runs the check without changing the version.
	unknown := a.createContact(token, map[string]any{"companyId": cimb.Id, "firstName": "Wei", "email": "wei@y.dnsdown.test"})
	res = a.request(http.MethodPost, "/v1/contacts/"+unknown.Id+"/email-check", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var rechecked api.Contact
	res.JSON(&rechecked)
	check, _ = rechecked.EmailCheck.Get()
	assert.Equal(t, api.EmailCheckStatusUnknown, check.Status)
	assert.Equal(t, unknown.Version, rechecked.Version)
}

func TestContactsListPermissionsAndIsolation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	maybank := a.createCompany(token, map[string]any{"name": "Maybank"})
	grab := a.createCompany(token, map[string]any{"name": "Grab"})
	a.createContact(token, map[string]any{"companyId": maybank.Id, "firstName": "José", "lastName": "Tan", "title": "Talent Acquisition"})
	a.createContact(token, map[string]any{"companyId": grab.Id, "firstName": "Mei", "email": "mei@grab.com"})

	var page api.ContactPage
	a.request(http.MethodGet, "/v1/contacts?q=jose", tenant.Users[api.RoleViewer].Token, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "José Tan", page.Items[0].FullName)
	assert.Equal(t, "Maybank", page.Items[0].CompanyName)

	a.request(http.MethodGet, "/v1/contacts?q=talent", token, nil).JSON(&page)
	assert.Len(t, page.Items, 1, "search includes the job title")

	a.request(http.MethodGet, "/v1/contacts?companyId="+grab.Id, token, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Mei", page.Items[0].FullName)
	status, _ := page.Items[0].EmailCheckStatus.Get()
	assert.Equal(t, api.EmailCheckStatusValid, status)

	res := a.request(http.MethodPost, "/v1/contacts", tenant.Users[api.RoleViewer].Token, map[string]any{"companyId": grab.Id, "firstName": "X"})
	assert.Equal(t, http.StatusForbidden, res.Status)

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.request(http.MethodGet, "/v1/contacts", other.Users[api.RoleOwner].Token, nil).JSON(&page)
	assert.Empty(t, page.Items)
}

// A contact's role (hiring manager, HR…) and whether they still work there:
// set, changed, cleared, and filtered on in the list.
func TestContactsRoleAndStatusFilters(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maybank"})
	hm := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Hana", "role": "hiring_manager"})
	hr := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Rina", "role": "hr", "status": "left"})
	unset := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Uma"})
	role, _ := hm.Role.Get()
	assert.Equal(t, "hiring_manager", role)
	assert.True(t, unset.Role.IsNull())

	names := func(query string) []string {
		var page api.ContactPage
		res := a.request(http.MethodGet, "/v1/contacts?"+query, token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&page)
		out := []string{}
		for _, c := range page.Items {
			out = append(out, c.FullName)
		}
		return out
	}
	assert.Equal(t, []string{"Hana"}, names("role=hiring_manager"))
	assert.Equal(t, []string{"Rina"}, names("role=hr"))
	assert.Equal(t, []string{"Uma"}, names("roleNotSet=true"))
	assert.Equal(t, []string{"Rina"}, names("status=left"))
	assert.Equal(t, []string{"Uma", "Hana"}, names("status=active"))
	assert.Empty(t, names("role=hr&status=active"))

	// Changing the role, then clearing it; the list shows the change.
	res := a.request(http.MethodPatch, "/v1/contacts/"+hr.Id, token, map[string]any{"version": hr.Version, "role": "hiring_manager", "status": "active"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var changed api.Contact
	res.JSON(&changed)
	role, _ = changed.Role.Get()
	assert.Equal(t, "hiring_manager", role)
	assert.Equal(t, []string{"Rina", "Hana"}, names("role=hiring_manager"))
	res = a.request(http.MethodPatch, "/v1/contacts/"+hr.Id, token, map[string]any{"version": changed.Version, "role": nil})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&changed)
	assert.True(t, changed.Role.IsNull())
	assert.Equal(t, []string{"Uma", "Rina"}, names("roleNotSet=true"))

	res = a.request(http.MethodPatch, "/v1/contacts/"+hm.Id, token, map[string]any{"version": hm.Version, "role": "Hiring Manager"})
	assert.Equal(t, http.StatusBadRequest, res.Status, "a role is a list key")
}

// The roles are the agency's own list in Settings, each with a colour.
func TestContactRolesAreAnAgencyList(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	owner := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	options := []map[string]any{
		{"key": "hiring_manager", "label": "Hiring manager", "color": "#2563eb"},
		{"key": "hr", "label": "HR / Talent acquisition", "color": "#DB2777"},
		{"label": "Director", "color": "#0d9488"},
	}
	res := a.request(http.MethodPut, "/v1/settings/fields/lists/contact_role", owner, map[string]any{"options": options})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var all api.FieldSettings
	a.request(http.MethodGet, "/v1/settings/fields", owner, nil).JSON(&all)
	var roles api.FieldList
	for _, l := range all.Lists {
		if l.Name == api.FieldListNameContactRole {
			roles = l
		}
	}
	require.Len(t, roles.Options, 3)
	assert.Equal(t, "HR / Talent acquisition", roles.Options[1].Label)
	assert.Equal(t, "#db2777", roles.Options[1].Color.MustGet())
	assert.Equal(t, "director", roles.Options[2].Key)

	company := a.createCompany(owner, map[string]any{"name": "Petronas"})
	c := a.createContact(owner, map[string]any{"companyId": company.Id, "firstName": "Dina", "role": "director"})
	assert.Equal(t, "director", c.Role.MustGet())
}
