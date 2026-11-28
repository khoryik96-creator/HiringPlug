package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// putRenamed saves a list as it is, with one word renamed (sent with its old
// word as its key, as the Settings page and the dropdowns do).
func (a *testApp) putRenamed(token string, name api.FieldListName, from, to string) *response {
	a.t.Helper()
	var all api.FieldSettings
	a.request(http.MethodGet, "/v1/settings/fields", token, nil).JSON(&all)
	var options []map[string]any
	for _, l := range all.Lists {
		if l.Name != name {
			continue
		}
		for _, o := range l.Options {
			label := o.Label
			if o.Key == from {
				label = to
			}
			options = append(options, map[string]any{"key": o.Key, "label": label, "hidden": o.Hidden})
		}
	}
	return a.request(http.MethodPut, "/v1/settings/fields/lists/"+string(name), token, map[string]any{"options": options})
}

// Renaming a specialization, IT skill or certification in Settings renames it
// on every candidate who has it (ignoring case; once where they already have
// the new word; binned ones too), and says so in the audit log.
func TestRenamingAListWordRenamesItOnCandidates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token

	aina := a.createCandidate(recruiter, map[string]any{"firstName": "Aina", "skills": []string{"Golang", "PHP"},
		"certifications": []string{"cisco", "ACCA"}, "specialization": "Marketing"})
	wei := a.createCandidate(recruiter, map[string]any{"firstName": "Wei", "skills": []string{"Go (Golang)", "Python", "golang"}})
	lama := a.createCandidate(recruiter, map[string]any{"firstName": "Lama", "certifications": []string{"CISCO"}})
	require.Equal(t, http.StatusCreated, a.bin(admin, map[string]any{"kind": "candidate", "id": lama.Id}).Status)

	for _, r := range []struct {
		list     api.FieldListName
		from, to string
	}{{api.FieldListNameItSkill, "Golang", "Go (Golang)"}, {api.FieldListNameCertification, "CISCO", "Cisco CCNA"},
		{api.FieldListNameSpecialization, "Marketing", "Digital Marketing"}} {
		res := a.putRenamed(admin, r.list, r.from, r.to)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}

	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+aina.Id, recruiter, nil).JSON(&got)
	assert.Equal(t, []string{"Go (Golang)", "PHP"}, got.Skills, "renamed where it was")
	assert.Equal(t, []string{"Cisco CCNA", "ACCA"}, got.Certifications, "ignoring case")
	assert.Equal(t, "Digital Marketing", got.Specialization.MustGet())
	assert.Greater(t, got.Version, aina.Version, "a form opened before shows a conflict instead of putting the old word back")
	a.request(http.MethodGet, "/v1/candidates/"+wei.Id, recruiter, nil).JSON(&got)
	assert.Equal(t, []string{"Go (Golang)", "Python"}, got.Skills, "the new word once, where it came first")

	var binned []string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT certifications FROM candidate WHERE id = $1`, lama.Id).Scan(&binned))
	assert.Equal(t, []string{"Cisco CCNA"}, binned, "a binned candidate comes back with the new word")

	entry := auditEntryOf(t, a.auditLog(admin, "?entityType=field_list"), "settings.list_changed", "certification")
	assert.Equal(t, []any{map[string]any{"from": "CISCO", "to": "Cisco CCNA", "records": float64(2)}}, entry.Changes["renamed"])
}

// Two words renamed into each other in one save could go either way: it is
// refused, and nothing changes.
func TestRenamingWordsIntoEachOtherIsRefused(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	c := a.createCandidate(owner, map[string]any{"firstName": "Aina", "certifications": []string{"CFA"}})
	res := a.request(http.MethodPut, "/v1/settings/fields/lists/certification", owner, map[string]any{"options": []map[string]any{
		{"key": "ACCA", "label": "CFA", "hidden": false}, {"key": "CFA", "label": "ACCA", "hidden": false},
	}})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+c.Id, owner, nil).JSON(&got)
	assert.Equal(t, []string{"CFA"}, got.Certifications)
}

// An admin whose role sees only their own candidates still renames the word
// on everyone's (the list is the company's), and in everyone's favourites;
// each candidate's own log shows it, and the suggestions have the new word.
func TestARenameReachesEveryCandidateAndFavourite(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	a.setRole(owner, "admin", map[string]any{"version": 0, "permissions": authz.PermissionsForRole(api.RoleAdmin), "candidateScope": "own", "jobCandidates": false}, http.StatusOK)

	theirs := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "skills": []string{"Golang"}})
	var used api.UsedValues
	a.request(http.MethodGet, "/v1/candidates/used-values", recruiter, nil).JSON(&used) // remembered with the old word
	res := a.request(http.MethodPut, "/v1/me/preferences", recruiter, map[string]any{"favourites": map[string]any{"itSkill": []string{"golang", "Java"}}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	res = a.putRenamed(admin, api.FieldListNameItSkill, "Golang", "Go")
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+theirs.Id, recruiter, nil).JSON(&got)
	assert.Equal(t, []string{"Go"}, got.Skills, "a candidate the admin cannot see is renamed too")
	var prefs api.Preferences
	a.request(http.MethodGet, "/v1/me/preferences", recruiter, nil).JSON(&prefs)
	require.NotNil(t, prefs.Favourites)
	require.NotNil(t, prefs.Favourites.ItSkill)
	assert.Equal(t, []string{"Go", "Java"}, *prefs.Favourites.ItSkill, "a colleague's star follows the word")

	entry := auditEntryOf(t, a.auditLog(owner, "?entityType=candidate&entityId="+theirs.Id), "candidate.updated", theirs.Id)
	assert.Equal(t, map[string]any{"from": "Golang", "to": "Go"}, entry.Changes["skills"])
	a.request(http.MethodGet, "/v1/candidates/used-values", recruiter, nil).JSON(&used)
	assert.Contains(t, used.Skills, "Go", "suggestions have the new word at once")
	assert.NotContains(t, used.Skills, "Golang")
}

// Taking a word out of a list, or hiding it, changes nothing for candidates
// who have it: they keep it, can still be saved, and are still found by it.
func TestRemovingOrHidingAWordLeavesCandidatesAsTheyAre(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(recruiter, map[string]any{"firstName": "Aina", "skills": []string{"PHP", "Python"},
		"certifications": []string{"CIMA"}, "specialization": "Marketing"})

	for _, l := range []struct {
		name  api.FieldListName
		gone  string
		hides string
	}{{api.FieldListNameItSkill, "PHP", "Python"}, {api.FieldListNameCertification, "CIMA", ""}, {api.FieldListNameSpecialization, "", "Marketing"}} {
		var all api.FieldSettings
		a.request(http.MethodGet, "/v1/settings/fields", owner, nil).JSON(&all)
		var options []map[string]any
		for _, list := range all.Lists {
			if list.Name != l.name {
				continue
			}
			for _, o := range list.Options {
				if o.Key != l.gone {
					options = append(options, map[string]any{"key": o.Key, "label": o.Label, "hidden": o.Key == l.hides})
				}
			}
		}
		res := a.request(http.MethodPut, "/v1/settings/fields/lists/"+string(l.name), owner, map[string]any{"options": options})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}

	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter, nil).JSON(&got)
	assert.Equal(t, []string{"PHP", "Python"}, got.Skills)
	assert.Equal(t, []string{"CIMA"}, got.Certifications)
	assert.Equal(t, "Marketing", got.Specialization.MustGet())
	assert.Equal(t, c.Version, got.Version, "untouched")

	res := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, recruiter, map[string]any{"version": got.Version, "currentTitle": "Developer"})
	require.Equal(t, http.StatusOK, res.Status, "a candidate with a word no longer offered still saves: %s", res.Body)
	res.JSON(&got)
	assert.Equal(t, []string{"PHP", "Python"}, got.Skills)

	var page api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates?skill=php", recruiter, nil).JSON(&page)
	require.Len(t, page.Items, 1, "still found by the word")
}
