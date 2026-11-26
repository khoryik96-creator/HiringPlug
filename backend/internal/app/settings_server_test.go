package app_test

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/buildinfo"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/heartbeat"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Settings > System's server checks: the agency that runs the server sees
// whether the workers run and on the API's version, whether files can be kept
// and opened, the disk, and what the alert emails report; others see none.
// (Not parallel: the workers' reports are the server's, one row each.)
func TestTheAgencyRunningTheServerSeesItsChecks(t *testing.T) {
	ctx := context.Background()
	op := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a := newTestApp(t, map[string]string{"HP_BACKUP_TENANT": op.ID})
	clear := func() { _, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM server_process`) }
	clear()
	t.Cleanup(clear)
	status := func(token string) api.SystemStatus {
		var st api.SystemStatus
		res := a.request(http.MethodGet, "/v1/settings/system", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&st)
		return st
	}
	owner := op.Users[api.RoleOwner].Token

	// Before any worker reported: nothing wrong, both "never seen".
	sc := status(owner).Server.MustGet()
	assert.Equal(t, buildinfo.Version(), sc.Version)
	require.Len(t, sc.Processes, 2)
	assert.Equal(t, api.ServerProcessStateNeverSeen, sc.Processes[0].State)
	assert.True(t, sc.Processes[0].SeenAt.IsNull())
	assert.True(t, sc.Files.IsNull(), "the files folder works: %v", sc.Files)
	assert.Positive(t, sc.FreeBytes.MustGet())
	assert.Empty(t, problemsAbout(sc.Problems, "worker"))

	// The worker reports as it runs; the AI worker is still on the old version.
	run, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		heartbeat.Run(run, testutil.Current.App, heartbeat.Worker, buildinfo.Version(), slog.New(slog.DiscardHandler))
	}()
	require.Eventually(t, func() bool {
		return status(owner).Server.MustGet().Processes[0].State == api.ServerProcessStateRunning
	}, 5*time.Second, 50*time.Millisecond)
	stop()
	<-done
	require.NoError(t, heartbeat.Beat(ctx, testutil.Current.App, heartbeat.AIWorker, "1.0.0", time.Now().Add(-time.Hour)))
	sc = status(op.Users[api.RoleAdmin].Token).Server.MustGet()
	assert.Equal(t, buildinfo.Version(), sc.Processes[0].Version.MustGet())
	ai := sc.Processes[1]
	assert.Equal(t, api.ServerProcessNameAiWorker, ai.Name)
	assert.Equal(t, api.ServerProcessStateOldVersion, ai.State)
	assert.Equal(t, "1.0.0", ai.Version.MustGet())
	assert.Equal(t, []string{"The AI worker still runs version 1.0.0, the rest of HiringPlug " + buildinfo.Version() +
		": restart it after the update."}, problemsAbout(sc.Problems, "worker"))

	// The AI worker stopped reporting.
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE server_process SET seen_at = now() - interval '5 minutes' WHERE name = 'ai-worker'`)
	require.NoError(t, err)
	sc = status(owner).Server.MustGet()
	assert.Equal(t, api.ServerProcessStateStopped, sc.Processes[1].State)
	about := problemsAbout(sc.Problems, "worker")
	require.Len(t, about, 1)
	assert.Contains(t, about[0], "The AI worker has not checked in since")

	// Another company's owner sees none of it.
	assert.True(t, status(other.Users[api.RoleOwner].Token).Server.IsNull())
}

// problemsAbout keeps the problems that mention word (other tests' failed
// jobs show too: the alert checks are the server's).
func problemsAbout(problems []string, word string) []string {
	var out []string
	for _, p := range problems {
		if strings.Contains(p, " "+word+" ") {
			out = append(out, p)
		}
	}
	return out
}
