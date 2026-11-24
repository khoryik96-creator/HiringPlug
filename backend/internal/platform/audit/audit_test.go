package audit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffListsChangedFields(t *testing.T) {
	city := "Ipoh"
	before := map[string]any{"name": "Maju", "city": (*string)(nil), "size": "small"}
	after := map[string]any{"name": "Maju Bhd", "city": &city, "size": "small"}
	assert.Equal(t, map[string]any{
		"name": map[string]any{"from": "Maju", "to": "Maju Bhd"},
		"city": map[string]any{"from": (*string)(nil), "to": &city},
	}, Diff(before, after))
}

// A record just made (Diff of an empty map) lists the fields it was made
// with, not an empty change; fields with no value are left out.
func TestDiffOfANewRecordListsItsFields(t *testing.T) {
	site := "https://maju.example"
	after := map[string]any{
		"name": "Maju Bhd", "website": &site, "about": (*string)(nil), "tags": []string(nil), "extra": map[string]any(nil), "none": nil, "openings": 0,
	}
	assert.Equal(t, map[string]any{
		"name":     map[string]any{"from": nil, "to": "Maju Bhd"},
		"website":  map[string]any{"from": nil, "to": &site},
		"openings": map[string]any{"from": nil, "to": 0},
	}, Diff(map[string]any{}, after))
}
