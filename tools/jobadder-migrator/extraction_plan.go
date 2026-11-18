package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Snapshot of all 240 GET operations in JobAdder's public v2 specification.
// Values from source records never become hosts or arbitrary request paths.
//
//go:embed extraction_routes.json
var routeJSON []byte

type RouteParam struct {
	Name, Type, Format string
	Enum               []string
}
type ExtractRoute struct {
	Path, Root, Family, Offset, Limit, Schema string
	Params                                    []RouteParam
	Query                                     url.Values
	Collection, Total, Binary                 bool
	PageSize                                  int
}

func extractionRoutes() []ExtractRoute {
	var routes []ExtractRoute
	if err := json.Unmarshal(routeJSON, &routes); err != nil {
		panic("invalid bundled route catalogue")
	}
	return routes
}

// Aliases use the paged global source, not an unpaged embedded child list.
// Their coverage is reported against that source even when it is denied.
func equivalentSource(r ExtractRoute) string {
	parts := strings.Split(strings.Trim(r.Path, "/"), "/")
	if len(parts) < 3 || !strings.HasPrefix(parts[1], "{") {
		return ""
	}
	child := parts[2]
	if len(parts) == 3 || (len(parts) == 4 && (parts[3] == "active" || parts[3] == "approved")) {
		switch child {
		case "notes", "applications", "placements", "submissions", "floats", "events", "interviews", "usertasks", "jobs", "contacts", "requisitions":
			return "/" + child
		case "activities":
			return "/" + r.Root + "/activities"
		}
	}
	if r.Path == "/candidates/{candidateId}/attachments" {
		return "/candidates/attachments"
	}
	return ""
}

func routeExcluded(r ExtractRoute) string {
	if r.Root == "search" {
		return "Matching helper; not a bulk data source"
	}
	if r.Path == "/candidates/payrollbilling" {
		return "Uses /candidates/{candidateId}/payrollbilling instead"
	}
	return ""
}

var integerID = regexp.MustCompile(`^[0-9]{1,19}$`)
var uuidID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validBinding(p RouteParam, value string) bool {
	if len(p.Enum) > 0 {
		for _, e := range p.Enum {
			if e == value {
				return true
			}
		}
		return false
	}
	if p.Type == "integer" {
		n, err := strconv.ParseUint(value, 10, 64)
		return err == nil && integerID.MatchString(value) && n > 0 && (p.Format != "int32" || n <= 2147483647)
	}
	return p.Format == "uuid" && uuidID.MatchString(value)
}

func makeTask(id int64, r ExtractRoute, bindings map[string]string) (ExtractTask, error) {
	path := r.Path
	scoped := map[string]string{}
	for _, p := range r.Params {
		v := bindings[p.Name]
		if !validBinding(p, v) {
			return ExtractTask{}, errors.New("missing or invalid source identifier")
		}
		scoped[p.Name] = v
		path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(v))
	}
	q := url.Values{}
	for k, values := range r.Query {
		q[k] = append([]string(nil), values...)
	}
	if r.Offset != "" {
		q.Set(r.Offset, "0")
		q.Set(r.Limit, strconv.Itoa(r.PageSize))
	}
	if q.Encode() != "" {
		path += "?" + q.Encode()
	}
	return ExtractTask{Export: id, Route: r.Path, Path: path, Bindings: scoped, Chain: taskKey(path)}, nil
}

func seedTasks(id int64, routes []ExtractRoute) []ExtractTask {
	var tasks []ExtractTask
	for _, r := range routes {
		if len(r.Params) != 0 || routeExcluded(r) != "" {
			continue
		}
		t, _ := makeTask(id, r, map[string]string{})
		// Defaults can hide retained records. Partition explicitly where supported.
		variants := []url.Values{{}}
		switch r.Path {
		case "/jobs", "/applications", "/opportunities":
			variants = []url.Values{{"Active": {"true"}}, {"Active": {"false"}}}
		case "/placements":
			variants = []url.Values{{"Approved": {"true"}, "Type": {"Permanent", "Contract", "Temporary", "Credit"}}, {"Approved": {"false"}, "Type": {"Permanent", "Contract", "Temporary", "Credit"}}}
		case "/folders":
			variants = []url.Values{{"OwnerStatus": {"Active"}}, {"OwnerStatus": {"Inactive"}}}
		}
		for _, v := range variants {
			u, _ := url.Parse(t.Path)
			q := u.Query()
			for k, values := range v {
				q[k] = values
			}
			u.RawQuery = q.Encode()
			next := t
			next.Path = u.String()
			next.Chain = taskKey(next.Path)
			tasks = append(tasks, next)
		}
	}
	// Fetch bulk collections before expanding children and native files.
	sort.SliceStable(tasks, func(i, j int) bool {
		return len(strings.Split(tasks[i].Route, "/")) < len(strings.Split(tasks[j].Route, "/"))
	})
	return tasks
}
