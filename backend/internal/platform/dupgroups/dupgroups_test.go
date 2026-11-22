package dupgroups

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinPutsRecordsFoundTwiceInOneGroup(t *testing.T) {
	got := Join([]Group{
		{Reason: "same_website", IDs: []string{"c_1", "c_2"}},
		{Reason: "similar_name", IDs: []string{"c_2", "c_3"}},
		{Reason: "similar_name", IDs: []string{"c_8", "c_9"}},
		{Reason: "same_linkedin", IDs: []string{"c_1", "c_2"}},
	})
	assert.Equal(t, []Joined{
		{Reasons: []string{"similar_name"}, IDs: []string{"c_9", "c_8"}},
		{Reasons: []string{"same_linkedin", "same_website", "similar_name"}, IDs: []string{"c_3", "c_2", "c_1"}},
	}, got, "newest first, each record once")
	assert.Empty(t, Join(nil))
}
