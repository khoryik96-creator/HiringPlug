package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/heartbeat"
)

// Process states.
const (
	Running    = "running"
	Stopped    = "stopped"     // it reported before, but not lately
	OldVersion = "old_version" // running, on another version than the API's
	NeverSeen  = "never_seen"  // never reported (not set up on this server, or before 1.52.0)
)

// ProcessState is a background process and how it is.
type ProcessState struct {
	heartbeat.Process
	State string
}

// processNames are the processes checked, in the order shown.
var processNames = []string{heartbeat.Worker, heartbeat.AIWorker}

// ProcessName is how a process is called in messages.
func ProcessName(name string) string {
	if name == heartbeat.AIWorker {
		return "The AI worker"
	}
	return "The background worker"
}

// Processes returns how each background process is, against c.Version.
func (c Checks) Processes(ctx context.Context) ([]ProcessState, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	seen, err := heartbeat.Read(ctx, c.DB)
	if err != nil {
		return nil, err
	}
	out := make([]ProcessState, 0, len(processNames))
	for _, name := range processNames {
		p, ok := seen[name]
		st := ProcessState{Process: p, State: Running}
		st.Name = name
		switch {
		case !ok:
			st.State = NeverSeen
		case now().Sub(p.SeenAt) > heartbeat.Stale:
			st.State = Stopped
		case known(c.Version) && known(p.Version) && p.Version != c.Version:
			st.State = OldVersion
		}
		out = append(out, st)
	}
	return out, nil
}

func known(v string) bool { return v != "" && v != "unknown" }

func processProblems(procs []ProcessState, version string) []Problem {
	var out []Problem
	for _, p := range procs {
		switch p.State {
		case Stopped:
			out = append(out, Problem{"process-stopped-" + p.Name, fmt.Sprintf(
				"%s has not checked in since %s: the work it does waits.", ProcessName(p.Name), p.SeenAt.Format(time.RFC1123))})
		case OldVersion:
			out = append(out, Problem{"process-version-" + p.Name, fmt.Sprintf(
				"%s still runs version %s, the rest of HyPlug %s: restart it after the update.", ProcessName(p.Name), p.Version, version)})
		}
	}
	return out
}
