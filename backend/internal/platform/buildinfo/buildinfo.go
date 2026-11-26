// Package buildinfo is the version of HiringPlug a program was built from
// (the VERSION file, raised with every update): Settings › System compares
// the workers' with the API's, so a worker still on the old version after an
// update shows.
package buildinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// version is set when an image is built:
// -ldflags "-X github.com/khoryik96-creator/hiringplug/backend/internal/platform/buildinfo.version=1.52.0".
var version string

var (
	once  sync.Once
	found string
)

var plain = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Version is the version built in or, in development (go run, go test), the
// VERSION file of the checkout the program runs in; "unknown" if neither.
func Version() string {
	once.Do(func() {
		found = strings.TrimSpace(version)
		if found == "" {
			found = fromCheckout()
		}
	})
	return found
}

func fromCheckout() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	for range 6 {
		if b, err := os.ReadFile(filepath.Join(dir, "VERSION")); err == nil {
			if v := strings.TrimSpace(string(b)); plain.MatchString(v) {
				return v
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "unknown"
}
