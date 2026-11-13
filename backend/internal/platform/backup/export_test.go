package backup

import (
	"errors"
	"fmt"
)

// Untar is untar, for tests.
var Untar = untar

// FindTool is toolFinder.tool with fakes, for tests.
func FindTool(name string, major int, onPath, pathVersion string, debianMajors ...int) (string, error) {
	f := toolFinder{
		lookPath: func(n string) (string, error) {
			if onPath == "" {
				return "", errors.New("not found")
			}
			return onPath + "/" + n, nil
		},
		exists: func(p string) bool {
			for _, m := range debianMajors {
				if p == fmt.Sprintf("/usr/lib/postgresql/%d/bin/%s", m, name) {
					return true
				}
			}
			return false
		},
		version: func(string) (string, error) { return name + " (PostgreSQL) " + pathVersion + "\n", nil },
	}
	return f.tool(name, major)
}

// LinkFiles and PreviousFiles are linkFiles and previousFiles, for tests.
var (
	LinkFiles     = linkFiles
	PreviousFiles = previousFiles
)
