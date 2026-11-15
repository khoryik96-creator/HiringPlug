package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) folder(method, path, token string, body any, want int) api.Folder {
	a.t.Helper()
	res := a.request(method, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var f api.Folder
	if want < 300 && want != http.StatusNoContent {
		res.JSON(&f)
	}
	return f
}

func (a *testApp) folders(token, kind string) map[string]api.Folder {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/folders?kind="+kind, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.FolderList
	res.JSON(&list)
	out := map[string]api.Folder{}
	for _, f := range list.Items {
		out[f.Name] = f
	}
	return out
}

func TestFoldersAreMadeRenamedAndDeletedByTheirOwner(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	me, colleague := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]

	f := a.folder(http.MethodPost, "/v1/folders", me.Token, map[string]any{"kind": "candidate", "name": "  Finance   shortlist "}, http.StatusCreated)
	assert.Equal(t, "Finance shortlist", f.Name)
	assert.Equal(t, me.ID, f.OwnerId)
	assert.True(t, f.Mine)
	assert.Empty(t, f.SharedWith)
	assert.Zero(t, f.ItemCount)
	a.folder(http.MethodPost, "/v1/folders", me.Token, map[string]any{"kind": "contact", "name": "HR in KL"}, http.StatusCreated)

	assert.Equal(t, []string{"Finance shortlist"}, keys(a.folders(me.Token, "candidate")), "each list has its own folders")
	assert.Equal(t, []string{"HR in KL"}, keys(a.folders(me.Token, "contact")))
	assert.Empty(t, a.folders(colleague.Token, "candidate"), "others do not see a folder until it is shared")

	res := a.request(http.MethodPost, "/v1/folders", me.Token, map[string]any{"kind": "candidate", "name": "   "})
	assert.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)

	renamed := a.folder(http.MethodPatch, "/v1/folders/"+f.Id, me.Token, map[string]any{"name": "Finance — final", "version": f.Version}, http.StatusOK)
	assert.Equal(t, "Finance — final", renamed.Name)
	assert.Greater(t, renamed.Version, f.Version)
	res = a.request(http.MethodPatch, "/v1/folders/"+f.Id, me.Token, map[string]any{"name": "Old copy", "version": f.Version})
	assert.Equal(t, http.StatusConflict, res.Status, "a rename at an old version is refused")

	res = a.request(http.MethodPatch, "/v1/folders/"+f.Id, colleague.Token, map[string]any{"name": "Mine now", "version": renamed.Version})
	assert.Equal(t, http.StatusNotFound, res.Status, "someone it is not shared with cannot even see it")

	c := a.createCandidate(me.Token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah"})
	a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", me.Token, map[string]any{"itemIds": []string{c.Id}}, http.StatusOK)
	a.folder(http.MethodDelete, "/v1/folders/"+f.Id, me.Token, nil, http.StatusNoContent)
	assert.Empty(t, a.folders(me.Token, "candidate"))
	res = a.request(http.MethodGet, "/v1/candidates/"+c.Id, me.Token, nil)
	assert.Equal(t, http.StatusOK, res.Status, "deleting a folder leaves the records in it")

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	assert.Empty(t, a.folders(other.Users[api.RoleOwner].Token, "contact"), "agencies never see each other's folders")
}

func TestFoldersHoldRecordsAndFilterTheLists(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	nurul := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah", "currentTitle": "Finance Manager"})
	wei := a.createCandidate(token, map[string]any{"firstName": "Wei", "lastName": "Tan", "currentTitle": "Finance Manager"})
	a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Hajar", "currentTitle": "Finance Manager"})
	f := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "candidate", "name": "Shortlist"}, http.StatusCreated)

	added := a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{nurul.Id, wei.Id, nurul.Id}}, http.StatusOK)
	assert.Equal(t, 2, added.ItemCount)
	again := a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{wei.Id}}, http.StatusOK)
	assert.Equal(t, 2, again.ItemCount, "adding twice keeps one")

	res := a.request(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{"cont_abc"}})
	assert.Equal(t, http.StatusBadRequest, res.Status, "a contact does not go into a candidate folder")

	var list api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates?folder="+f.Id, token, nil).JSON(&list)
	assert.ElementsMatch(t, []string{nurul.Id, wei.Id}, candidateIDs(list.Items))

	var page api.CandidateSearchPage
	res = a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"boolean": "finance", "folderId": f.Id})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&page)
	got := []string{}
	for _, h := range page.Items {
		got = append(got, h.Candidate.Id)
	}
	assert.ElementsMatch(t, []string{nurul.Id, wei.Id}, got, "a search can look only in a folder")

	var member api.FolderMemberships
	a.request(http.MethodPost, "/v1/folders/memberships", token, map[string]any{"kind": "candidate", "itemIds": []string{nurul.Id}}).JSON(&member)
	require.Len(t, member.Items, 1)
	assert.Equal(t, f.Id, member.Items[0].FolderId)
	assert.Equal(t, []string{nurul.Id}, member.Items[0].ItemIds)

	left := a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items/remove", token, map[string]any{"itemIds": []string{wei.Id}}, http.StatusOK)
	assert.Equal(t, 1, left.ItemCount)
	a.request(http.MethodGet, "/v1/candidates?folder="+f.Id, token, nil).JSON(&list)
	assert.Equal(t, []string{nurul.Id}, candidateIDs(list.Items))

	contacts := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "contact", "name": "HR"}, http.StatusCreated)
	res = a.request(http.MethodGet, "/v1/candidates?folder="+contacts.Id, token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status, "a contact folder does not filter candidates")
}

func TestContactAndCompanyFoldersFilterTheirLists(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	acme := a.createCompany(token, map[string]any{"name": "Acme Foods"})
	maju := a.createCompany(token, map[string]any{"name": "Maju Holdings"})
	ana := a.createContact(token, map[string]any{"companyId": acme.Id, "firstName": "Ana"})
	a.createContact(token, map[string]any{"companyId": maju.Id, "firstName": "Ben"})

	people := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "contact", "name": "Warm"}, http.StatusCreated)
	a.folder(http.MethodPost, "/v1/folders/"+people.Id+"/items", token, map[string]any{"itemIds": []string{ana.Id}}, http.StatusOK)
	var contacts api.ContactPage
	a.request(http.MethodGet, "/v1/contacts?folder="+people.Id, token, nil).JSON(&contacts)
	require.Len(t, contacts.Items, 1)
	assert.Equal(t, ana.Id, contacts.Items[0].Id)

	firms := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "company", "name": "Targets"}, http.StatusCreated)
	a.folder(http.MethodPost, "/v1/folders/"+firms.Id+"/items", token, map[string]any{"itemIds": []string{maju.Id}}, http.StatusOK)
	var companies api.CompanyPage
	a.request(http.MethodGet, "/v1/companies?folder="+firms.Id, token, nil).JSON(&companies)
	require.Len(t, companies.Items, 1)
	assert.Equal(t, maju.Id, companies.Items[0].Id)

	res := a.request(http.MethodGet, "/v1/companies?folder="+people.Id, token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status, "a contact folder does not filter companies")

	// Taking records out of a folder, or deleting it, never deletes the records.
	nurul := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah"})
	pool := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "candidate", "name": "Pool"}, http.StatusCreated)
	a.folder(http.MethodPost, "/v1/folders/"+pool.Id+"/items", token, map[string]any{"itemIds": []string{nurul.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+people.Id+"/items/remove", token, map[string]any{"itemIds": []string{ana.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+firms.Id+"/items/remove", token, map[string]any{"itemIds": []string{maju.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+pool.Id+"/items/remove", token, map[string]any{"itemIds": []string{nurul.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+pool.Id+"/items", token, map[string]any{"itemIds": []string{nurul.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+firms.Id+"/items", token, map[string]any{"itemIds": []string{maju.Id}}, http.StatusOK)
	for _, id := range []string{people.Id, firms.Id, pool.Id} {
		a.folder(http.MethodDelete, "/v1/folders/"+id, token, nil, http.StatusNoContent)
	}
	for _, path := range []string{"/v1/candidates/" + nurul.Id, "/v1/contacts/" + ana.Id, "/v1/companies/" + maju.Id, "/v1/companies/" + acme.Id} {
		assert.Equal(t, http.StatusOK, a.request(http.MethodGet, path, token, nil).Status, "%s is still there", path)
	}
	var all api.ContactPage
	a.request(http.MethodGet, "/v1/contacts", token, nil).JSON(&all)
	assert.Len(t, all.Items, 2, "both contacts are still listed")
}

func TestSharedFoldersLetColleaguesAddAndRemoveButOnlyTheOwnerManages(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer)
	owner, sourcer, viewer := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer], tenant.Users[api.RoleViewer]
	c := a.createCandidate(owner.Token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah"})
	f := a.folder(http.MethodPost, "/v1/folders", owner.Token, map[string]any{"kind": "candidate", "name": "Team pool"}, http.StatusCreated)

	shared := a.folder(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+sourcer.ID, owner.Token, nil, http.StatusOK)
	assert.Equal(t, []string{sourcer.ID}, shared.SharedWith)
	shared = a.folder(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+viewer.ID, owner.Token, nil, http.StatusOK)
	assert.ElementsMatch(t, []string{sourcer.ID, viewer.ID}, shared.SharedWith)
	a.folder(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+viewer.ID, owner.Token, nil, http.StatusOK) // again: no change
	res := a.request(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+owner.ID, owner.Token, nil)
	assert.Equal(t, http.StatusBadRequest, res.Status, "the owner already has it")
	res = a.request(http.MethodPut, "/v1/folders/"+f.Id+"/shares/usr_00000000000000000000000000", owner.Token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status, "only people of the agency")
	elsewhere := testutil.Current.NewTenant(t, api.RoleOwner)
	res = a.request(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+elsewhere.Users[api.RoleOwner].ID, owner.Token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status, "not someone of another agency")

	seen := a.folders(sourcer.Token, "candidate")["Team pool"]
	assert.False(t, seen.Mine)
	assert.Equal(t, owner.ID, seen.OwnerId)
	added := a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", sourcer.Token, map[string]any{"itemIds": []string{c.Id}}, http.StatusOK)
	assert.Equal(t, 1, added.ItemCount, "those it is shared with add records")
	assert.Equal(t, 1, a.folders(owner.Token, "candidate")["Team pool"].ItemCount, "and the owner sees them")
	a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items/remove", sourcer.Token, map[string]any{"itemIds": []string{c.Id}}, http.StatusOK)
	renamed := a.folder(http.MethodPatch, "/v1/folders/"+f.Id, owner.Token, map[string]any{"name": "Team pool 2", "version": f.Version}, http.StatusOK)
	assert.Equal(t, "Team pool 2", renamed.Name, "records coming and going never make the owner's rename stale")
	seen.Version = renamed.Version

	for name, try := range map[string]func() *response{
		"rename": func() *response {
			return a.request(http.MethodPatch, "/v1/folders/"+f.Id, sourcer.Token, map[string]any{"name": "Mine", "version": seen.Version})
		},
		"delete": func() *response { return a.request(http.MethodDelete, "/v1/folders/"+f.Id, sourcer.Token, nil) },
		"share": func() *response {
			return a.request(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+sourcer.ID, sourcer.Token, nil)
		},
		"remove someone else": func() *response {
			return a.request(http.MethodDelete, "/v1/folders/"+f.Id+"/shares/"+viewer.ID, sourcer.Token, nil)
		},
	} {
		res := try()
		require.Equal(t, http.StatusForbidden, res.Status, "%s: %s", name, res.Body)
		assert.Equal(t, api.ErrorCodeFOLDEROWNERONLY, res.Error().Error.Code, name)
	}

	var gone api.FolderOrGone
	res = a.request(http.MethodDelete, "/v1/folders/"+f.Id+"/shares/"+viewer.ID, viewer.Token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&gone)
	assert.True(t, gone.Folder.IsNull(), "someone may leave a folder shared with them")
	assert.Empty(t, a.folders(viewer.Token, "candidate"))

	res = a.request(http.MethodDelete, "/v1/folders/"+f.Id+"/shares/"+sourcer.ID, owner.Token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&gone)
	assert.Empty(t, gone.Folder.MustGet().SharedWith)
	assert.Empty(t, a.folders(sourcer.Token, "candidate"), "once removed, they no longer see it")
	res = a.request(http.MethodPost, "/v1/folders/"+f.Id+"/items", sourcer.Token, map[string]any{"itemIds": []string{c.Id}})
	assert.Equal(t, http.StatusNotFound, res.Status)
}

func TestMergingCandidatesKeepsTheirFolders(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	token := tenant.Users[api.RoleAdmin].Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah"})
	dup := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "A."})
	both := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "candidate", "name": "Both"}, http.StatusCreated)
	onlyDup := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "candidate", "name": "Dup only"}, http.StatusCreated)
	a.folder(http.MethodPost, "/v1/folders/"+both.Id+"/items", token, map[string]any{"itemIds": []string{keep.Id, dup.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+onlyDup.Id+"/items", token, map[string]any{"itemIds": []string{dup.Id}}, http.StatusOK)

	res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	all := a.folders(token, "candidate")
	assert.Equal(t, 1, all["Both"].ItemCount, "the merged pair counts once")
	assert.Equal(t, 1, all["Dup only"].ItemCount)
	var list api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates?folder="+onlyDup.Id, token, nil).JSON(&list)
	assert.Equal(t, []string{keep.Id}, candidateIDs(list.Items), "the kept candidate takes the duplicate's place")
}

func keys(m map[string]api.Folder) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func candidateIDs(items []api.CandidateSummary) []string {
	out := []string{}
	for _, c := range items {
		out = append(out, c.Id)
	}
	return out
}

func TestFoldersShowOnlyTheCandidatesSomeoneMaySee(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleSourcer)
	owner, sourcer := tenant.Users[api.RoleOwner], tenant.Users[api.RoleSourcer]
	a.setRole(owner.Token, "sourcer", map[string]any{"version": 0, "permissions": authz.PermissionsForRole(api.RoleSourcer), "candidateScope": "own", "jobCandidates": false}, http.StatusOK)
	theirs := a.createCandidate(sourcer.Token, map[string]any{"firstName": "Siti", "lastName": "Hajar"})
	mine := a.createCandidate(owner.Token, map[string]any{"firstName": "Wei", "lastName": "Tan"})
	other := a.createCandidate(owner.Token, map[string]any{"firstName": "Arjun", "lastName": "Mehta"})
	f := a.folder(http.MethodPost, "/v1/folders", owner.Token, map[string]any{"kind": "candidate", "name": "Pool"}, http.StatusCreated)
	a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", owner.Token, map[string]any{"itemIds": []string{theirs.Id, mine.Id, other.Id}}, http.StatusOK)
	a.folder(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+sourcer.ID, owner.Token, nil, http.StatusOK)

	assert.Equal(t, 1, a.folders(sourcer.Token, "candidate")["Pool"].ItemCount, "the count never reveals candidates they may not see")
	var list api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates?folder="+f.Id, sourcer.Token, nil).JSON(&list)
	assert.Equal(t, []string{theirs.Id}, candidateIDs(list.Items))
	var member api.FolderMemberships
	a.request(http.MethodPost, "/v1/folders/memberships", sourcer.Token, map[string]any{"kind": "candidate", "itemIds": []string{mine.Id, theirs.Id}}).JSON(&member)
	require.Len(t, member.Items, 1)
	assert.Equal(t, []string{theirs.Id}, member.Items[0].ItemIds)

	mineToo := a.folder(http.MethodPost, "/v1/folders", sourcer.Token, map[string]any{"kind": "candidate", "name": "Mine"}, http.StatusCreated)
	res := a.request(http.MethodPost, "/v1/folders/"+mineToo.Id+"/items", sourcer.Token, map[string]any{"itemIds": []string{mine.Id}})
	assert.Equal(t, http.StatusNotFound, res.Status, "a candidate they may not see does not go in")
	got := a.folder(http.MethodPost, "/v1/folders/"+mineToo.Id+"/items", sourcer.Token, map[string]any{"itemIds": []string{mine.Id, theirs.Id}}, http.StatusOK)
	assert.Equal(t, 1, got.ItemCount, "only those they may see")
	assert.Equal(t, 3, a.folders(owner.Token, "candidate")["Pool"].ItemCount, "the owner, who sees everyone, sees all three")
}

func TestFoldersTakeOnlyRealRecordsAndLogWhatChanged(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	nurul := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah"})
	wei := a.createCandidate(token, map[string]any{"firstName": "Wei", "lastName": "Tan"})
	f := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "candidate", "name": "Shortlist"}, http.StatusCreated)

	res := a.request(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{"cand_00000000000000000000000000"}})
	assert.Equal(t, http.StatusNotFound, res.Status, "a made-up candidate never goes in")
	elsewhere := testutil.Current.NewTenant(t, api.RoleRecruiter)
	stranger := a.createCandidate(elsewhere.Users[api.RoleRecruiter].Token, map[string]any{"firstName": "Ali", "lastName": "Hassan"})
	got := a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{nurul.Id, stranger.Id}}, http.StatusOK)
	assert.Equal(t, 1, got.ItemCount, "nor one of another agency")

	a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{nurul.Id, wei.Id}}, http.StatusOK)
	a.folder(http.MethodPost, "/v1/folders/"+f.Id+"/items/remove", token, map[string]any{"itemIds": []string{wei.Id, "cand_00000000000000000000000000"}}, http.StatusOK)
	logged := func(action string) []string {
		rows, err := testutil.Current.Admin.Query(context.Background(),
			`SELECT changes->'items'->>'ids' FROM audit_log WHERE entity_id = $1 AND action = $2 ORDER BY created_at`, f.Id, action)
		require.NoError(t, err)
		out, err := pgx.CollectRows(rows, pgx.RowTo[string])
		require.NoError(t, err)
		return out
	}
	assert.Equal(t, []string{`["` + nurul.Id + `"]`, `["` + wei.Id + `"]`}, logged("folder.items_added"), "only what was put in, not what was already there")
	assert.Equal(t, []string{`["` + wei.Id + `"]`}, logged("folder.items_removed"), "only what was taken out")
}
