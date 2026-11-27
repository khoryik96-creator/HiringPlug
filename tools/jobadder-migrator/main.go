// HyPlug Migration: standalone, read-only JobAdder discovery. No HyPlug
// database, API, Python worker or Docker dependency is required.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	defaultDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("user workspace folder unavailable")
	}
	dir := flag.String("workspace", filepath.Join(defaultDir, "HiringPlugMigration"), "persistent, private workspace folder")
	port := flag.Int("port", 8769, "local browser port; must match the JobAdder app callback")
	demo := flag.Bool("demo", false, "use synthetic data; never connect to JobAdder")
	noBrowser := flag.Bool("no-browser", false, "open the private launch shortcut manually")
	flag.Parse()
	if *port < 1024 || *port > 65535 {
		return fmt.Errorf("choose a local port between 1024 and 65535")
	}
	if *demo {
		*dir = filepath.Join(*dir, "demo")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(*port))
	if err != nil {
		return fmt.Errorf("local port unavailable; the tool may already be open")
	}
	defer listener.Close()
	store, err := OpenStore(*dir)
	if err != nil {
		return fmt.Errorf("workspace could not be opened; choose a writable private folder")
	}
	defer store.Close()
	a := &App{store: store, origin: "http://localhost:" + strconv.Itoa(*port), session: randomID(), clientHTTP: secureHTTP(), pace: 500 * time.Millisecond, demo: *demo}
	if *demo {
		if err = store.Unlock("demo workspace synthetic only"); err != nil {
			return err
		}
		a.unlocked = true
		a.connection = Connection{API: "https://api.jobadder.com/v2", AccessToken: "synthetic", Account: "Sample agency — DEMO", Fingerprint: "synthetic-account"}
		a.clientHTTP = &http.Client{Transport: demoTransport{}}
		a.pace = 25 * time.Millisecond
	}
	launch := filepath.Join(*dir, "HyPlug Migration.url")
	if err = os.WriteFile(launch, []byte("[InternetShortcut]\nURL="+a.origin+"/#"+a.session+"\n"), 0600); err != nil {
		return fmt.Errorf("private browser launch shortcut could not be created")
	}
	defer os.Remove(launch)
	fmt.Println("HyPlug Migration is running locally. Open the HyPlug Migration.url shortcut in your workspace if the browser does not open.")
	if *demo {
		fmt.Println("DEMO: synthetic records only. No JobAdder or HyPlug connection.")
	}
	if !*noBrowser {
		openBrowser(a.origin + "/#" + a.session)
	}
	requestContext, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	server := &http.Server{Handler: a.Handler(), BaseContext: func(net.Listener) context.Context { return requestContext }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: exportWriteTimeout, IdleTimeout: 60 * time.Second}
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-signals.Done():
		case <-done:
		}
		cancelRequests()
		a.mu.Lock()
		a.closing = true
		if a.cancel != nil {
			a.cancel()
		}
		a.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if server.Shutdown(ctx) != nil {
			_ = server.Close()
		}
	}()
	err = server.Serve(listener)
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	close(done)
	<-shutdownDone
	// HTTP handlers and inventory workers finish before their store is closed.
	a.work.Wait()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("local browser server stopped unexpectedly")
	}
	return nil
}
func openBrowser(address string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		cmd = exec.Command("open", address)
	default:
		cmd = exec.Command("xdg-open", address)
	}
	if cmd.Start() == nil {
		go func() { _ = cmd.Wait() }()
	}
}

type demoTransport struct{}

func (demoTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("demo refuses writes")
	}
	code := 200
	var data any = map[string]any{"items": []any{}, "totalCount": 12}
	switch strings.TrimPrefix(r.URL.Path, "/v2") {
	case "/candidates":
		data = map[string]any{"items": []any{}, "totalCount": 80000}
	case "/notes":
		data = map[string]any{"items": []any{}, "totalCount": 240000}
	case "/contacts":
		data = map[string]any{"items": []any{}, "totalCount": 12000}
	case "/companies":
		data = map[string]any{"items": []any{}, "totalCount": 4000}
	case "/candidates/attachments":
		data = map[string]any{"items": []any{}, "totalCount": 96000}
	case "/opportunities":
		code = 403
		data = map[string]string{"error": "demo unavailable"}
	case "/categories", "/countries", "/locations", "/worktypes":
		data = map[string]any{"items": []any{map[string]any{"name": "Original source label", "value": "Keep unchanged"}}}
	default:
		if strings.Contains(r.URL.Path, "/fields/") || strings.Contains(r.URL.Path, "/lists/") {
			data = map[string]any{"items": []any{map[string]any{"name": "Original source label", "value": "Keep unchanged"}}}
		}
	}
	raw, _ := json.Marshal(data)
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
}
