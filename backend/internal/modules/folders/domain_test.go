package folders

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamesAreTidiedAndChecked(t *testing.T) {
	name, errs := NormalizeName("  Finance   shortlist ")
	assert.Empty(t, errs)
	assert.Equal(t, "Finance shortlist", name)
	_, errs = NormalizeName("   ")
	assert.Len(t, errs, 1)
	_, errs = NormalizeName(strings.Repeat("a", 101))
	assert.Len(t, errs, 1)
}

func TestAFolderHoldsOnlyItsKind(t *testing.T) {
	cand := "cand_01jbz6w0m0k8q9x2r3s4t5v6w7"
	ids, errs := ItemIDs(Candidates, []string{cand, cand})
	assert.Empty(t, errs)
	assert.Equal(t, []string{cand}, ids, "once each")
	_, errs = ItemIDs(Contacts, []string{cand})
	assert.Len(t, errs, 1, "a candidate is not a contact")
	_, errs = ItemIDs("job", []string{cand})
	assert.Len(t, errs, 1)
}
