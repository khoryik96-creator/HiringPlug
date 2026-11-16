package app_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Colleagues adding to one shared folder at the same moment (or one person
// dropping rows twice quickly) all get through: changes to a folder take turns.
func TestManyAddToOneFolderAtOnce(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	owner, colleague := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]
	f := a.folder(http.MethodPost, "/v1/folders", owner.Token, map[string]any{"kind": "candidate", "name": "Busy"}, http.StatusCreated)
	a.folder(http.MethodPut, "/v1/folders/"+f.Id+"/shares/"+colleague.ID, owner.Token, nil, http.StatusOK)
	var ids []string
	for range 12 {
		ids = append(ids, a.createCandidate(owner.Token, map[string]any{"firstName": "Nurul", "lastName": "Aisyah"}).Id)
	}
	statuses := make([]int, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		token := owner.Token
		if i%2 == 1 {
			token = colleague.Token
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = a.request(http.MethodPost, "/v1/folders/"+f.Id+"/items", token, map[string]any{"itemIds": []string{id}}).Status
		}()
	}
	wg.Wait()
	for i, s := range statuses {
		assert.Equal(t, http.StatusOK, s, "addition %d", i)
	}
	assert.Equal(t, len(ids), a.folders(owner.Token, "candidate")["Busy"].ItemCount)
}

// Delete clicked twice: one deletes it, the other finds it gone, and the log
// says it was deleted once.
func TestAFolderIsDeletedOnce(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := a.folder(http.MethodPost, "/v1/folders", token, map[string]any{"kind": "contact", "name": "Old"}, http.StatusCreated)
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = a.request(http.MethodDelete, "/v1/folders/"+f.Id, token, nil).Status
		}()
	}
	wg.Wait()
	assert.ElementsMatch(t, []int{http.StatusNoContent, http.StatusNotFound}, statuses)
	var logged int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'folder.deleted'`, f.Id).Scan(&logged))
	assert.Equal(t, 1, logged)
}
