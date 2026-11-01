// Package scaffold creates a new, working module from templates: its
// contract file, migration (with row-level security), SQL queries, Go code,
// tests, and the wiring in internal/app/modules.go.
//
// The generated module passes every check as soon as `task gen` has run, so
// a new module starts from the same patterns and guardrails as the others.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templates embed.FS

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	prefixRe = regexp.MustCompile(`^[a-z]{2,8}$`)
	numberRe = regexp.MustCompile(`^(\d{5})_`)
)

// Names are every spelling of the module the templates need.
type Names struct {
	Module  string // contract name and permission prefix: job_ads
	Package string // Go package and directory: jobads
	Path    string // URL path segment: job-ads
	Plural  string // operation and field names: JobAds
	Entity  string // schema and type names: JobAd
	Table   string // database table: job_ad
	Prefix  string // ID prefix (2-8 letters): jobad -> jobad_01...
	Human   string // for descriptions: job ad
}

// NewNames derives all spellings from the module name (plural) and the
// entity name (singular). prefix may be empty to derive one.
func NewNames(module, entity, prefix string) (Names, error) {
	if !nameRe.MatchString(module) || !nameRe.MatchString(entity) {
		return Names{}, errors.New("name and entity must be lower_snake_case, e.g. name=job_ads entity=job_ad")
	}
	if prefix == "" {
		prefix = strings.ReplaceAll(entity, "_", "")
		prefix = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r
			}
			return -1
		}, prefix)
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
	}
	if !prefixRe.MatchString(prefix) {
		return Names{}, fmt.Errorf("ID prefix %q must be 2-8 lower-case letters (set one with prefix=xyz)", prefix)
	}
	return Names{
		Module:  module,
		Package: strings.ReplaceAll(module, "_", ""),
		Path:    strings.ReplaceAll(module, "_", "-"),
		Plural:  pascal(module),
		Entity:  pascal(entity),
		Table:   entity,
		Prefix:  prefix,
		Human:   strings.ReplaceAll(entity, "_", " "),
	}, nil
}

func pascal(snake string) string {
	parts := strings.Split(snake, "_")
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// Generate creates the module under repoRoot and returns the paths it wrote.
func Generate(repoRoot string, n Names) ([]string, error) {
	backend := filepath.Join(repoRoot, "backend")
	moduleDir := filepath.Join(backend, "internal", "modules", n.Package)
	contract := filepath.Join(repoRoot, "contracts", "modules", n.Module+".yaml")
	for _, p := range []string{moduleDir, contract} {
		if _, err := os.Stat(p); err == nil {
			return nil, fmt.Errorf("%s already exists", p)
		}
	}
	migration, err := nextMigration(filepath.Join(backend, "db", "migrations"), n.Module)
	if err != nil {
		return nil, err
	}

	files := map[string]string{
		"contract.yaml.tmpl":  contract,
		"migration.sql.tmpl":  migration,
		"queries.sql.tmpl":    filepath.Join(backend, "db", "queries", n.Module+".sql"),
		"module.go.tmpl":      filepath.Join(moduleDir, n.Package+".go"),
		"domain.go.tmpl":      filepath.Join(moduleDir, "domain.go"),
		"domain_test.go.tmpl": filepath.Join(moduleDir, "domain_test.go"),
		"service.go.tmpl":     filepath.Join(moduleDir, "service.go"),
		"http.go.tmpl":        filepath.Join(moduleDir, "http.go"),
		"app_test.go.tmpl":    filepath.Join(backend, "internal", "app", n.Package+"_test.go"),
	}
	var written []string
	for tmpl, dest := range files {
		content, err := render(tmpl, n)
		if err != nil {
			return written, err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return written, err
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil { //nolint:gosec // source files
			return written, err
		}
		written = append(written, dest)
	}
	if err := appendSQLC(filepath.Join(backend, "sqlc.yaml"), n); err != nil {
		return written, err
	}
	if err := wireModule(filepath.Join(backend, "internal", "app", "modules.go"), n); err != nil {
		return written, err
	}
	sort.Strings(written)
	return append(written, filepath.Join(backend, "sqlc.yaml"), filepath.Join(backend, "internal", "app", "modules.go")), nil
}

func render(name string, n Names) ([]byte, error) {
	t, err := template.New(name).Delims("[[", "]]").ParseFS(templates, "templates/"+name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, n); err != nil {
		return nil, err
	}
	if strings.HasSuffix(name, ".go.tmpl") {
		return format.Source(buf.Bytes())
	}
	return buf.Bytes(), nil
}

func nextMigration(dir, module string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	highest := 0
	for _, e := range entries {
		if m := numberRe.FindStringSubmatch(e.Name()); m != nil {
			var n int
			_, _ = fmt.Sscanf(m[1], "%d", &n)
			highest = max(highest, n)
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%05d_%s.sql", highest+1, module)), nil
}

func appendSQLC(path string, n Names) error {
	data, err := os.ReadFile(path) //nolint:gosec // the repo's own config
	if err != nil {
		return err
	}
	block := fmt.Sprintf("  - <<: *module\n    queries: db/queries/%s.sql\n    gen:\n      go:\n        <<: *go\n        out: internal/modules/%s/internal/store\n", n.Module, n.Package)
	return os.WriteFile(path, append(data, []byte(block)...), 0o644) //nolint:gosec // source file
}

// wireModule inserts the module at the `// scaffold:...` markers.
func wireModule(path string, n Names) error {
	data, err := os.ReadFile(path) //nolint:gosec // the repo's own source
	if err != nil {
		return err
	}
	s := string(data)
	inserts := []struct{ marker, line string }{
		{"\t// scaffold:imports", fmt.Sprintf("\t\"github.com/khoryik96-creator/hiringplug/backend/internal/modules/%s\"", n.Package)},
		{"\t// scaffold:fields", fmt.Sprintf("\t%s *%s.Module", n.Plural, n.Package)},
		{"\t// scaffold:constructors", fmt.Sprintf("\tm.%s = %s.New(d.Pool)", n.Plural, n.Package)},
		{"\t// scaffold:aliases", fmt.Sprintf("\t%sHTTP = %s.HTTP", n.Package, n.Package)},
		{"\t// scaffold:embeds", fmt.Sprintf("\t*%sHTTP", n.Package)},
		{"\t\t// scaffold:server-fields", fmt.Sprintf("\t\t%sHTTP: m.%s.HTTP,", n.Package, n.Plural)},
	}
	for _, in := range inserts {
		if !strings.Contains(s, in.marker+"\n") {
			return fmt.Errorf("%s: marker %q not found; wire the module by hand", path, strings.TrimSpace(in.marker))
		}
		s = strings.Replace(s, in.marker+"\n", in.line+"\n"+in.marker+"\n", 1)
	}
	formatted, err := format.Source([]byte(sortModuleImports(s)))
	if err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0o644) //nolint:gosec // source file
}

// sortModuleImports keeps the module imports above the marker sorted.
func sortModuleImports(s string) string {
	lines := strings.Split(s, "\n")
	end := -1
	for i, l := range lines {
		if l == "\t// scaffold:imports" {
			end = i
			break
		}
	}
	start := end
	for start > 0 && strings.HasPrefix(lines[start-1], "\t\"github.com/khoryik96-creator/hiringplug/backend/internal/modules/") {
		start--
	}
	if end > start {
		group := append([]string(nil), lines[start:end]...)
		sort.Strings(group)
		copy(lines[start:end], group)
	}
	return strings.Join(lines, "\n")
}
