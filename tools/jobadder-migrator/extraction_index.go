package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var sourceEntities = map[string]string{
	"candidateId": "candidates", "companyId": "companies", "contactId": "contacts", "jobId": "jobs", "jobOrderId": "jobs",
	"applicationId": "applications", "placementId": "placements", "requisitionId": "requisitions", "noteId": "notes",
	"folderId": "folders", "userId": "users", "groupId": "usergroups", "officeId": "useroffices", "taskId": "usertasks",
	"adId": "jobads", "boardId": "jobboards", "floatId": "floats", "submissionId": "submissions", "eventId": "events",
	"interviewId": "interviews", "opportunityId": "opportunities", "actionId": "partners", "webhookId": "webhooks",
}

type RecordRef struct{ Entity, ID, Pointer string }
type EdgeRef struct {
	From, To          RecordRef
	Relation, Pointer string
}
type PageIndex struct {
	Records []RecordRef
	Edges   []EdgeRef
	Tasks   []ExtractTask
}

func scalarID(v any) string {
	switch n := v.(type) {
	case json.Number:
		return n.String()
	case string:
		if len(n) <= 128 {
			return n
		}
	}
	return ""
}
func pointerToken(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
func cloneBindings(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Walk actual associations in one object/ancestor chain. Never take a Cartesian
// product of identifiers from unrelated collection items or sibling objects.
func indexPage(t ExtractTask, r ExtractRoute, value any, routes []ExtractRoute) PageIndex {
	var out PageIndex
	seenTasks := map[string]bool{}
	var walk func(any, string, map[string]string, *RecordRef, string, int)
	walk = func(v any, pointer string, parent map[string]string, owner *RecordRef, relation string, depth int) {
		if depth > 64 {
			return
		} // JSON decoding separately rejects excessive depth.
		switch node := v.(type) {
		case []any:
			for i, child := range node {
				walk(child, pointer+"/"+strconv.Itoa(i), parent, owner, relation, depth+1)
			}
		case map[string]any:
			bindings := cloneBindings(parent)
			var direct []RecordRef
			hasLocal := false
			keys := make([]string, 0, len(node))
			for key := range node {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if id := scalarID(node[key]); id != "" {
					if entity, ok := sourceEntities[key]; ok {
						bindings[key] = id
						direct = append(direct, RecordRef{entity, id, pointer})
					} else if strings.HasSuffix(key, "Id") || key == "videoType" || key == "workflowType" {
						bindings[key] = id
						hasLocal = true
					}
				}
			}
			if id := bindings["attachmentId"]; id != "" {
				bindings["attach"] = id
			}
			if id := bindings["jobId"]; id != "" {
				bindings["jobOrderId"] = id
			}
			if id := bindings["jobOrderId"]; id != "" {
				bindings["jobId"] = id
			}
			if id := bindings["actionId"]; id != "" {
				bindings["act"] = id
			}
			var current *RecordRef = owner
			for i := range direct {
				ref := direct[i]
				out.Records = append(out.Records, ref)
				if owner != nil && (owner.Entity != ref.Entity || owner.ID != ref.ID) {
					out.Edges = append(out.Edges, EdgeRef{*owner, ref, relation, pointer})
				}
				if current == owner || ref.Entity == r.Root {
					copy := ref
					current = &copy
				}
			}
			// Same-object identifiers express a relationship (e.g. application -> candidate).
			if current != nil {
				for _, ref := range direct {
					if ref.Entity != current.Entity || ref.ID != current.ID {
						out.Edges = append(out.Edges, EdgeRef{*current, ref, relation, pointer})
					}
				}
			}
			if len(direct) > 0 || hasLocal {
				for _, route := range routes {
					if len(route.Params) == 0 || equivalentSource(route) != "" || routeExcluded(route) != "" {
						continue
					}
					// Local IDs such as fieldId/attachmentId/statusId only belong to their
					// source family. A contact's field 1 is not a candidate's field 1.
					local := false
					for _, p := range route.Params {
						if _, ok := sourceEntities[p.Name]; !ok && p.Name != "jobOrderId" {
							local = true
						}
					}
					if local && route.Root != r.Root {
						continue
					}
					variants := []map[string]string{bindings}
					for _, param := range route.Params {
						if bindings[param.Name] == "" && len(param.Enum) > 0 {
							var expanded []map[string]string
							for _, b := range variants {
								for _, e := range param.Enum {
									next := cloneBindings(b)
									next[param.Name] = e
									expanded = append(expanded, next)
								}
							}
							variants = expanded
						}
					}
					for _, b := range variants {
						next, err := makeTask(t.Export, route, b)
						if err == nil && !seenTasks[next.Path] {
							seenTasks[next.Path] = true
							out.Tasks = append(out.Tasks, next)
						}
					}
				}
			}
			for _, key := range keys {
				if key == "entityIDs" && current != nil {
					if groups, ok := node[key].(map[string]any); ok {
						for entity, ids := range groups {
							if list, ok := ids.([]any); ok {
								for i, id := range list {
									value := scalarID(id)
									if value != "" {
										out.Edges = append(out.Edges, EdgeRef{*current, RecordRef{entity, value, pointer}, "folder membership", fmt.Sprintf("%s/entityIDs/%s/%d", pointer, pointerToken(entity), i)})
									}
								}
							}
						}
					}
				}
				walk(node[key], pointer+"/"+pointerToken(key), bindings, current, key, depth+1)
			}
		}
	}
	walk(value, "", t.Bindings, nil, "source", 0)
	return out
}
