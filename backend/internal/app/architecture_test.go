package app_test

// Architecture rules, checked on every `go test`. They keep modules
// independent so that adding or changing one cannot break another.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const modulePath = "github.com/khoryik96-creator/hiringplug/backend/"

type goFile struct {
	path    string // relative to backend/, e.g. internal/modules/candidates/domain.go
	imports []string
	lines   int
}

func sourceFiles(t *testing.T) []goFile {
	t.Helper()
	root := "../.."
	var files []goFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "tools/") || strings.HasSuffix(rel, ".gen.go") || strings.Contains(rel, "/internal/store/") {
			return nil // generated code
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path) //nolint:gosec // reading this repository's own source
		if err != nil {
			return err
		}
		f := goFile{path: rel, lines: strings.Count(string(content), "\n")}
		for _, imp := range parsed.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			f.imports = append(f.imports, p)
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func moduleOf(path string) string {
	rest, ok := strings.CutPrefix(path, "internal/modules/")
	if !ok {
		return ""
	}
	return strings.SplitN(rest, "/", 2)[0]
}

func TestModulesOnlyMeetInTheAppPackage(t *testing.T) {
	for _, f := range sourceFiles(t) {
		mine := moduleOf(f.path)
		if mine == "" {
			continue
		}
		for _, imp := range f.imports {
			other := moduleOf(strings.TrimPrefix(imp, modulePath))
			assert.True(t, other == "" || other == mine,
				"%s imports module %q: modules must not import each other; define a small interface and wire it in internal/app/modules.go", f.path, other)
		}
	}
}

func TestPlatformDoesNotDependOnModules(t *testing.T) {
	for _, f := range sourceFiles(t) {
		if !strings.HasPrefix(f.path, "internal/platform/") || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		for _, imp := range f.imports {
			rel := strings.TrimPrefix(imp, modulePath)
			assert.False(t, strings.HasPrefix(rel, "internal/modules/") || strings.HasPrefix(rel, "internal/app"),
				"%s imports %s: platform code serves every module and must not depend on one", f.path, imp)
		}
	}
}

func TestDomainRulesArePure(t *testing.T) {
	forbidden := []string{"github.com/jackc/", "net/http", modulePath + "internal/platform/database", modulePath + "internal/platform/jobqueue", modulePath + "internal/api"}
	for _, f := range sourceFiles(t) {
		if moduleOf(f.path) == "" || filepath.Base(f.path) != "domain.go" {
			continue
		}
		for _, imp := range f.imports {
			for _, bad := range forbidden {
				assert.False(t, strings.HasPrefix(imp, bad),
					"%s imports %s: domain.go holds pure rules (no database, HTTP or contract types) so it stays easy to test", f.path, imp)
			}
		}
	}
}

func TestFilesStaySmall(t *testing.T) {
	const limit = 500
	for _, f := range sourceFiles(t) {
		assert.LessOrEqual(t, f.lines, limit, "%s has %d lines; split it (big files are where regressions hide)", f.path, f.lines)
	}
}
