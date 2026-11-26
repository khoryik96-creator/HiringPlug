package settings

// Settings › System's server checks, for the agency that runs the server:
// whether the workers run and on the API's version, whether the API can keep
// and open files, the files' disk, and what the alert emails report now.

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/alerts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/buildinfo"
)

// ServerSetup is what the server checks look at.
type ServerSetup struct {
	FilesDir string                     // HP_FILES_DIR: its disk's space
	Files    interface{ Check() error } // the uploaded files: written, read back and removed
}

// UseServer gives Settings the server's files for its checks.
func (m *Module) UseServer(s ServerSetup) { m.service.server = s }

// ServerChecks is how the server is.
type ServerChecks struct {
	Version     string
	Processes   []alerts.ProcessState
	Files       error // nil: files can be kept and opened
	Free, Total uint64
	SpaceKnown  bool
	Problems    []alerts.Problem
}

func (s *Service) serverChecks(ctx context.Context, tx pgx.Tx) (*ServerChecks, error) {
	checks := alerts.Checks{DB: tx, FilesDir: s.server.FilesDir, Version: buildinfo.Version()}
	out := ServerChecks{Version: checks.Version}
	var err error
	if out.Processes, err = checks.Processes(ctx); err != nil {
		return nil, err
	}
	if out.Problems, err = checks.Problems(ctx); err != nil {
		return nil, err
	}
	if s.server.Files != nil {
		out.Files = s.server.Files.Check()
	}
	out.Free, out.Total, out.SpaceKnown = checks.Space()
	return &out, nil
}

func serverToAPI(st *ServerChecks, out *api.SystemStatus) {
	out.Server.SetNull()
	if st == nil {
		return
	}
	sc := api.ServerChecks{Version: st.Version, Processes: []api.ServerProcess{}, Problems: []string{}}
	for _, p := range st.Processes {
		sp := api.ServerProcess{Name: api.ServerProcessName(p.Name), State: api.ServerProcessState(p.State)}
		sp.Version.SetNull()
		sp.StartedAt.SetNull()
		sp.SeenAt.SetNull()
		if p.State != alerts.NeverSeen {
			sp.Version.Set(p.Version)
			sp.StartedAt.Set(p.StartedAt)
			sp.SeenAt.Set(p.SeenAt)
		}
		sc.Processes = append(sc.Processes, sp)
	}
	sc.Files.SetNull()
	if st.Files != nil {
		sc.Files.Set(st.Files.Error())
	}
	sc.FreeBytes.SetNull()
	sc.TotalBytes.SetNull()
	if st.SpaceKnown {
		sc.FreeBytes.Set(bytesOf(st.Free))
		sc.TotalBytes.Set(bytesOf(st.Total))
	}
	for _, p := range st.Problems {
		sc.Problems = append(sc.Problems, p.Text)
	}
	out.Server.Set(sc)
}

func bytesOf(n uint64) int64 {
	if n > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(n)
}
