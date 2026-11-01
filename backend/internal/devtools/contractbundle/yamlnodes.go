package contractbundle

// Small helpers for editing YAML documents while keeping their key order, so
// the generated contract reads in the same order as the files it comes from.

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

func readYAML(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path) //nolint:gosec // dev tool reading the repo's own contract files
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &doc, nil
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	return doc.Content[0]
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalarAt(m *yaml.Node, key string) string {
	if n := mapGet(m, key); n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}

// mappingEntries iterates a mapping node's key/value pairs in document order.
func mappingEntries(m *yaml.Node) func(yield func(string, *yaml.Node) bool) {
	return func(yield func(string, *yaml.Node) bool) {
		if m == nil || m.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			if !yield(m.Content[i].Value, m.Content[i+1]) {
				return
			}
		}
	}
}

func mapAppend(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func mapSet(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	mapAppend(m, key, value)
}

func ensureMapping(m *yaml.Node, key string) *yaml.Node {
	if n := mapGet(m, key); n != nil && n.Kind == yaml.MappingNode {
		n.Style = 0 // an empty `{}` placeholder must not force flow style on the merged result
		return n
	}
	n := &yaml.Node{Kind: yaml.MappingNode}
	mapSet(m, key, n)
	return n
}

func setEnum(schema *yaml.Node, values []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range values {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	mapSet(schema, "enum", seq)
}

func encodeNode(v any) *yaml.Node {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		panic(err) // only called with plain structs defined in this file
	}
	return &n
}

func stripComments(n *yaml.Node) {
	n.HeadComment, n.LineComment, n.FootComment = "", "", ""
	for _, c := range n.Content {
		stripComments(c)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
