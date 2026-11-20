package app_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// bannerDraftProvider is the Outlook draft fixture with the drafts' files:
// what was attached (inline or not) and removed.
type bannerDraftProvider struct {
	*draftProvider
	files map[string][]map[string]any
}

func (f *bannerDraftProvider) serve(w http.ResponseWriter, r *http.Request) {
	id, route, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/me/messages/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/me/messages/") || !strings.HasPrefix(route, "attachments") {
		f.draftProvider.serve(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	draft := f.drafts[id]
	if draft == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case route == "attachments" && r.Method == http.MethodPost:
		var file map[string]any
		_ = json.NewDecoder(r.Body).Decode(&file)
		file["id"] = "att-" + file["name"].(string)
		f.files[id] = append(f.files[id], file)
		f.updateVersion(draft)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(file)
	case route == "attachments" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{"value": f.files[id]})
	case strings.HasPrefix(route, "attachments/") && r.Method == http.MethodDelete:
		at := slices.IndexFunc(f.files[id], func(file map[string]any) bool { return file["id"] == strings.TrimPrefix(route, "attachments/") })
		if at < 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.files[id] = slices.Delete(f.files[id], at, at+1)
		f.updateVersion(draft)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *bannerDraftProvider) filesOf(id string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any{}, f.files[id]...)
}

func (f *bannerDraftProvider) body(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := f.drafts[id]["body"].(map[string]any)
	s, _ := b["content"].(string)
	return s
}

func uploadTestBanner(t *testing.T, a *testApp, admin string) string {
	t.Helper()
	res := a.upload("/v1/email-banners", admin, "", "hyppies-banner.png", bannerPNG)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var b api.EmailBanner
	res.JSON(&b)
	return b.Id
}

func TestEachPersonHasTheirOwnEmailSignature(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	banner := uploadTestBanner(t, a, admin)

	var sig api.EmailSignature
	a.request("GET", "/v1/email-signature", recruiter, nil).JSON(&sig)
	assert.Equal(t, api.EmailSignature{BodyHtml: "", Version: 0, UpdatedAt: sig.UpdatedAt}, sig, "none until written")

	res := a.request("PUT", "/v1/email-signature", recruiter, map[string]any{"version": 0,
		"bodyHtml": `<div data-signature="true"><p><strong>Nadia Aziz</strong> · Senior Consultant</p><script>steal()</script>` +
			`<p><a href="https://hyppies.com">hyppies.com</a></p><img data-banner-id="` + banner + `" alt="Hyppies"></div>`})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&sig)
	assert.Equal(t, 1, sig.Version)
	assert.Contains(t, sig.BodyHtml, "<strong>Nadia Aziz</strong>")
	assert.Contains(t, sig.BodyHtml, `data-banner-id="`+banner+`"`)
	assert.Contains(t, sig.BodyHtml, `data-signature="true"`)
	assert.NotContains(t, sig.BodyHtml, "steal")
	assert.Equal(t, http.StatusConflict, a.request("PUT", "/v1/email-signature", recruiter, map[string]any{"version": 0, "bodyHtml": "<p>Old</p>"}).Status)

	var theirs api.EmailSignature
	a.request("GET", "/v1/email-signature", admin, nil).JSON(&theirs)
	assert.Empty(t, theirs.BodyHtml, "a signature is its owner's alone")

	// A banner deleted since cannot be kept in it.
	require.Equal(t, http.StatusNoContent, a.request("DELETE", "/v1/email-banners/"+banner, admin, nil).Status)
	res = a.request("PUT", "/v1/email-signature", recruiter, map[string]any{"version": 1, "bodyHtml": `<p>Nadia</p><img data-banner-id="` + banner + `">`})
	assert.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
}

func TestBannersInOutlookDraftsAreSentWithThem(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	banner := uploadTestBanner(t, a, admin)
	f := &bannerDraftProvider{draftProvider: newDraftProvider(), files: map[string][]map[string]any{}}
	connectTestOutlook(t, a, recruiter, http.HandlerFunc(f.serve))
	require.Equal(t, http.StatusNoContent, a.request("POST", "/v1/outlook/mail", recruiter, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	withBanner := `<p>Hi Samuel,</p><div data-signature="true"><p>Nadia</p><img data-banner-id="` + banner + `" src="blob:https://app.test/x" alt="Hyppies"></div>`
	res := a.request("POST", "/v1/outlook/drafts", recruiter, map[string]any{"to": []string{"samuel@client.test"}, "subject": "Profiles", "bodyHtml": withBanner})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var draft api.OutlookDraft
	res.JSON(&draft)
	remote := "draft-id-0"
	files := f.filesOf(remote)
	require.Len(t, files, 1, "the banner is attached to the draft")
	assert.Equal(t, banner, files[0]["contentId"])
	assert.Equal(t, true, files[0]["isInline"])
	assert.Equal(t, "hp-banner-"+banner+".png", files[0]["name"])
	assert.Contains(t, f.body(remote), `src="cid:`+banner+`"`, "the message shows the attached picture")
	assert.NotContains(t, f.body(remote), "blob:")
	assert.Contains(t, f.body(remote), `data-signature="true"`)
	require.NotNil(t, draft.BodyHtml)
	assert.Contains(t, *draft.BodyHtml, `data-banner-id="`+banner+`"`, "the editor shows it again")

	// Saved again with it: not attached twice. Taken out: removed.
	res = a.request("PATCH", "/v1/outlook/drafts/"+draft.MessageId, recruiter, map[string]any{"version": draft.Version, "bodyHtml": withBanner + "<p>PS</p>"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&draft)
	assert.Len(t, f.filesOf(remote), 1)
	res = a.request("PATCH", "/v1/outlook/drafts/"+draft.MessageId, recruiter, map[string]any{"version": draft.Version, "bodyHtml": "<p>Hi Samuel,</p>"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&draft)
	assert.Empty(t, f.filesOf(remote))

	// Scheduled with the banner put back: attached before it goes.
	res = a.request("PUT", "/v1/outlook/drafts/"+draft.MessageId+"/send", recruiter, map[string]any{
		"draftVersion": draft.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour), "changes": map[string]string{"bodyHtml": withBanner},
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	assert.Len(t, f.filesOf(remote), 1)

	// A reply quoting a banner deleted since, or another company's, is saved:
	// the history is left as it is and nothing is attached for it.
	require.Equal(t, http.StatusNoContent, a.request("DELETE", "/v1/email-banners/"+banner, admin, nil).Status)
	foreign := "ebn_01jbz6w0m0k8q9x2r3s4t5v6w7"
	quoted := `<p>Thanks!</p><hr><blockquote><img data-banner-id="` + banner + `"><img data-banner-id="` + foreign + `"></blockquote>`
	res = a.request("POST", "/v1/outlook/drafts", recruiter, map[string]any{"subject": "RE: Profiles", "bodyHtml": quoted})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	assert.Empty(t, f.filesOf("draft-id-"+lastDraftSequence(f)), "no picture attached for a deleted or foreign banner")
}

// lastDraftSequence is the number in the id the fixture gave the newest draft.
func lastDraftSequence(f *bannerDraftProvider) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	newest := ""
	for id := range f.drafts {
		if len(id) > len(newest) || id > newest {
			newest = id
		}
	}
	return strings.TrimPrefix(newest, "draft-id-")
}
