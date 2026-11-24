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

// A record just made (Diff of an empty map) is no change: webhook payloads
// copy these changes and must not carry a new record's every field.
func TestDiffOfANewRecordIsEmpty(t *testing.T) {
	assert.Empty(t, Diff(map[string]any{}, map[string]any{"name": "Maju Bhd", "notes": "private"}))
}
