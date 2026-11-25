package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Review of 1.47.0: ending a dead reading looked for a reading under way,
// then changed the document; a reading asked for in between (not committed
// yet when it looked) was then overwritten: the CV said its text could not
// be read while its new reading was waiting. The document is locked first
// now, so the end waits for the new reading and leaves it alone.
func TestAReadingEndingAsANewOneIsAskedForLeavesTheNewOne(t *testing.T) {
	t.Parallel()
	for name, meanwhile := range map[string]string{
		// A reading asked for again: its job queued.
		"asked again": `INSERT INTO job (tenant_id, queue, kind, payload) VALUES ($1, 'ai', 'document.extract_text', jsonb_build_object('documentId', $2::text))`,
		// Its text came in after all: the reading had ended already.
		"ended already": `UPDATE document SET status = 'extracted', extracted_text = 'Zara Lee, accountant' WHERE id = $2 AND tenant_id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := endWhile(t, meanwhile)
			if name == "asked again" {
				assert.Equal(t, api.DocumentStatusQueued, got.Status, "the new reading decides")
				assert.Equal(t, api.DocumentReadingJobWaiting, got.ReadingJob.MustGet())
			} else {
				assert.Equal(t, api.DocumentStatusExtracted, got.Status, "left as it is, without an error")
			}
		})
	}
}

// endWhile fails a new CV's reading for good, and has the Go worker end it
// while another transaction, doing meanwhile ($1 the company, $2 the
// document), holds the document; it returns the document once both are done.
func endWhile(t *testing.T, meanwhile string) api.Document {
	t.Helper()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	})
	c := a.createCandidate(token, map[string]any{"firstName": "Zara"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", pdfWith(c.Id))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	failAIJob(t, conn, claimAIJobFor(t, conn, doc.Id), true, "RuntimeError: killed")

	other, err := testutil.Current.Admin.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = other.Rollback(context.Background()) }()
	var pid int
	require.NoError(t, other.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	_, err = other.Exec(ctx, `UPDATE document SET updated_at = now() WHERE id = $1`, doc.Id)
	require.NoError(t, err)
	_, err = other.Exec(ctx, meanwhile, tenant.ID, doc.Id)
	require.NoError(t, err)

	w, err := app.NewWorker(a.deps, a.modules)
	require.NoError(t, err)
	w.Tenant = tenant.ID
	done := make(chan error, 1)
	go func() {
		_, err := w.RunOnce(ctx)
		done <- err
	}()
	require.Eventually(t, func() bool {
		var waiting int
		_ = testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY (pg_blocking_pids(pid))`, pid).Scan(&waiting)
		return waiting > 0
	}, 10*time.Second, 10*time.Millisecond, "the worker waits for the document")
	require.NoError(t, other.Commit(ctx))
	require.NoError(t, <-done)
	var failed int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'background_job.ended'
		AND status <> 'done'`, tenant.ID).Scan(&failed))
	assert.Zero(t, failed, "ending it raised no error")
	return a.getDocument(token, doc.Id)
}

// Review of 1.47.0: the upkeep took the oldest documents left being read
// first, so a few it could not set right (an error each time) came first
// on every run, and the others were never reached. One it could not set
// right waits a day; the next run reaches the others.
func TestTheUpkeepIsNotHeldUpByADocumentItCannotSetRight(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Lim"})
	var broken, fine api.Document
	for i, d := range []*api.Document{&broken, &fine} {
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", pdfWith(c.Id+[]string{"x", "y"}[i]))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		res.JSON(d)
	}
	admin := testutil.Current.Admin
	_, err := admin.Exec(ctx, `DELETE FROM job WHERE payload->>'documentId' = ANY ($1)`, []string{broken.Id, fine.Id})
	require.NoError(t, err)
	// Left long ago (older than any other test's), the broken one first; it
	// cannot be changed (as a document whose change fails for good).
	_, err = admin.Exec(ctx, `UPDATE document SET updated_at = now() - interval '20 years' - CASE WHEN id = $1 THEN interval '1 day' ELSE interval '0' END
		WHERE id = ANY ($2)`, broken.Id, []string{broken.Id, fine.Id})
	require.NoError(t, err)
	trigger := "test_unchangeable_" + broken.Id[len(broken.Id)-8:]
	_, err = admin.Exec(ctx, `CREATE FUNCTION `+trigger+`() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.id = '`+broken.Id+`' THEN RAISE EXCEPTION 'this document cannot be changed'; END IF; RETURN NEW; END $$;
		CREATE TRIGGER `+trigger+` BEFORE UPDATE ON document FOR EACH ROW EXECUTE FUNCTION `+trigger+`()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+trigger+` ON document; DROP FUNCTION IF EXISTS `+trigger+`()`)
	})

	for range 2 {
		_, err := a.modules.Documents.ReconcileReading(ctx, 1)
		require.NoError(t, err)
	}
	assert.Equal(t, api.DocumentStatusFailed, a.getDocument(token, fine.Id).Status, "reached on the second run")
	assert.Equal(t, api.DocumentStatusQueued, a.getDocument(token, broken.Id).Status)
}
