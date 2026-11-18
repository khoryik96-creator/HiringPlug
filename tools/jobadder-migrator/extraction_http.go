package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (a *App) startExtraction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Resume bool `json:"resume"`
	}
	if !input(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.unlocked || a.running || a.transferring || a.connecting || a.closing || a.connection.Fingerprint == "" {
		failure(w, "Connect JobAdder and pause other work first")
		return
	}
	until, err := a.store.RetryUntil()
	if err != nil {
		workspaceFailure(w)
		return
	}
	if time.Now().Before(until) {
		reply(w, 429, map[string]string{"error": (&CooldownError{Until: until}).Error()})
		return
	}
	id, err := a.store.BeginExtraction(a.connection.Fingerprint, in.Resume)
	if err != nil {
		failure(w, "Extraction checkpoint could not be opened")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.current = "Starting extraction"
	a.mode = "extraction"
	a.message = ""
	c := a.client(a.connection)
	a.work.Add(1)
	go func() {
		defer a.work.Done()
		err := extract(ctx, a.store, c, id, extractionRoutes(), func(label string) { a.mu.Lock(); a.current = label; a.mu.Unlock() })
		status, message := "api_pass_finished", "API pass finished. Review coverage, inaccessible resources and provider-export requirements. This is not a verified full-account backup."
		if err != nil {
			status = "paused"
			message = "Extraction paused. Completed checkpoints are saved; check the connection and resume."
			var cooldown *CooldownError
			if errors.As(err, &cooldown) {
				message = cooldown.Error()
			}
			if errors.Is(err, context.Canceled) {
				message = "Paused. Completed checkpoints are saved."
			}
		}
		if _, err := a.store.db.Exec(`UPDATE extractions SET status=?,finished=? WHERE id=?`, status, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			message = "Extraction stopped; final status could not be saved."
		}
		a.mu.Lock()
		a.running = false
		a.current = ""
		a.message = message
		a.mu.Unlock()
	}()
	reply(w, 202, map[string]bool{"ok": true})
}

func (a *App) sourceUpload(w http.ResponseWriter, r *http.Request) {
	if !a.beginTransfer() {
		failure(w, "Unlock the workspace and pause other work first")
		return
	}
	defer a.endTransfer()
	name := r.URL.Query().Get("name")
	entity := r.URL.Query().Get("entity")
	idField := r.URL.Query().Get("idField")
	kind := r.URL.Query().Get("source")
	if name == "" || len(name) > 1024 || strings.ContainsRune(name, 0) || !sourceLabel.MatchString(entity) || len(idField) > 128 || (kind != "jobadder_backup" && kind != "mailbox" && kind != "other") {
		failure(w, "Choose a file and valid source type")
		return
	}
	// Large uploads stream directly to encrypted chunks. Stop stalled transfers,
	// but do not impose the normal short JSON request timeout on a backup.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(2 * time.Minute))
	_ = controller.SetWriteDeadline(time.Time{})
	r.Body = http.MaxBytesReader(w, r.Body, maxSourceBytes)
	b, err := a.store.PutBlob(r.Context(), &uploadReader{r.Body, controller})
	if err != nil {
		failure(w, "Upload interrupted or could not be saved. Retry the file.")
		return
	}
	id, err := a.store.addImport(b, name, kind)
	if err != nil {
		workspaceFailure(w)
		return
	}
	_ = a.store.ingestSource(r.Context(), id, b, name, entity, idField)
	reply(w, 201, map[string]any{"id": id, "blob": b, "originalPreserved": true})
}

type uploadReader struct {
	reader     interface{ Read([]byte) (int, error) }
	controller *http.ResponseController
}

func (r *uploadReader) Read(p []byte) (int, error) {
	_ = r.controller.SetReadDeadline(time.Now().Add(2 * time.Minute))
	return r.reader.Read(p)
}

func (a *App) extractionReport(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	unlocked := a.unlocked
	a.mu.Unlock()
	if !unlocked {
		failure(w, "Unlock the workspace")
		return
	}
	report, err := a.store.extractionReport()
	if err != nil {
		workspaceFailure(w)
		return
	}
	reply(w, 200, report)
}

func (a *App) beginTransfer() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.unlocked || a.running || a.transferring || a.connecting || a.closing {
		return false
	}
	a.transferring = true
	return true
}
func (a *App) endTransfer() { a.mu.Lock(); a.transferring = false; a.mu.Unlock() }
