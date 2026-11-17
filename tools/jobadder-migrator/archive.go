package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

const exportWriteTimeout = 2 * time.Minute

var archiveKey = regexp.MustCompile(`^[a-z0-9_]+$`)

func copyContext(ctx context.Context, w io.Writer, r io.Reader) error {
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(buffer)
		if n > 0 {
			written, writeErr := w.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Build and verify the complete ZIP before sending headers. The private staging
// file is removed when the request ends; it never holds the application mutex.
func (s *Store) prepareArchive(ctx context.Context, report Report, observations []Observation, demo bool) (*os.File, error) {
	current := map[string]int64{}
	for _, result := range report.Results {
		if result.Digest == "" {
			continue
		}
		for _, o := range observations {
			if o.Key == result.Key && o.Digest == result.Digest && o.Checked == result.EvidenceChecked {
				current[result.Key] = o.ID
			}
		}
		if current[result.Key] == 0 {
			return nil, errors.New("saved response metadata failed verification")
		}
	}
	f, err := createArchiveFile()
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	z := zip.NewWriter(f)
	write := func(name string, raw []byte) error {
		entry, err := z.Create(name)
		if err != nil {
			return err
		}
		return copyContext(ctx, entry, bytes.NewReader(raw))
	}
	for _, o := range observations {
		if !archiveKey.MatchString(o.Key) {
			return nil, errors.New("invalid saved resource key")
		}
		if o.Digest == "" {
			continue
		}
		raw, err := s.ObservationOriginal(o.ID)
		if err != nil {
			return nil, err
		}
		if err := write("observations/"+o.Key+"/"+strconv.FormatInt(o.ID, 10)+".json", raw); err != nil {
			return nil, err
		}
		if current[o.Key] == o.ID {
			if err := write("responses/"+o.Key+".json", raw); err != nil {
				return nil, err
			}
		}
	}
	manifest, err := json.Marshal(map[string]any{"demo": demo, "report": report, "observations": observations})
	if err != nil {
		return nil, err
	}
	if err := write("manifest.json", manifest); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	success = true
	return f, nil
}

func (a *App) archiveSnapshot() (Report, []Observation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.unlocked || a.running || a.closing {
		return Report{}, nil, errors.New("Pause the inventory before exporting original responses")
	}
	report, err := a.store.Report()
	if err != nil {
		return Report{}, nil, errors.New("Report could not be read")
	}
	if report.Status == "running" {
		report.Status = "paused"
	}
	observations, err := a.store.Observations(report.Run)
	if err != nil {
		return Report{}, nil, errors.New("Saved response history could not be read")
	}
	return report, observations, nil
}

func (a *App) originals(w http.ResponseWriter, r *http.Request) {
	report, observations, err := a.archiveSnapshot()
	if err != nil {
		failure(w, err.Error())
		return
	}
	f, err := a.store.prepareArchive(r.Context(), report, observations, a.demo)
	if err != nil {
		failure(w, "Original responses could not be verified or staged; no archive was exported")
		return
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	info, err := f.Stat()
	if err != nil {
		failure(w, "Archive could not be read")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(exportWriteTimeout))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="jobadder-original-responses.zip"`)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_ = copyContext(r.Context(), w, f)
}
