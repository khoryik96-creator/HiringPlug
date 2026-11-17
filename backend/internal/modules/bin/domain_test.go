package bin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnApplicationIsShownAgainOnlyWhenItsCandidateAndJobAreBothBack(t *testing.T) {
	apps := []Application{{ID: "a1", CandidateID: "c1", JobID: "j1"}, {ID: "a2", CandidateID: "c1", JobID: "j2"}, {ID: "a3", CandidateID: "c2", JobID: "j1"}}
	assert.Equal(t, []string{"a1"}, unbinnable(apps, []string{"c2"}, []string{"j2"}))
	assert.Empty(t, unbinnable(apps, []string{"c1", "c2"}, nil))
}

func TestWhatWentWithACompanyIsKept(t *testing.T) {
	w := WentWith{Contacts: []string{"ct_1"}, Jobs: []string{"job_1", "job_2"}}
	assert.Equal(t, w, decodeWentWith(w.encode()))
	assert.Equal(t, WentWith{}, decodeWentWith([]byte(`{}`)))
	assert.Contains(t, placedReason(2), "2 placements")
}
