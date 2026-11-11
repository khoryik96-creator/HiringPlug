package industries

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func testCatalogue(t *testing.T) []api.Industry {
	t.Helper()
	var seed []seedCategory
	require.NoError(t, json.Unmarshal(seedData, &seed))
	out := []api.Industry{}
	for _, g := range seed {
		source := g.ID
		out = append(out, api.Industry{Id: g.ID, Label: g.Label, SourceKey: &source})
		for _, c := range g.Children {
			source := c.ID
			parent := g.ID
			out = append(out, api.Industry{Id: c.ID, Label: c.Label, SourceKey: &source, ParentId: &parent})
		}
	}
	return out
}
func TestCanonicalMatchingAndHierarchy(t *testing.T) {
	items := testCatalogue(t)
	require.Len(t, items, 91)
	assert.Equal(t, "fsi-banking", Key(" FSI  – Banking "))
	require.NotNil(t, Match(items, " fsi - banking "))
	assert.Nil(t, Match(items, "unknown"))
	alias := "finance"
	items[0].Aliases = []string{alias}
	items[1].Aliases = []string{alias}
	assert.Nil(t, Match(items, alias))
	_, e := CheckEdit(items, "", api.IndustryEdit{Label: "x", ParentId: &items[1].Id})
	require.Error(t, e)
}
func TestRecognitionPreservesVerifiedSector(t *testing.T) {
	items := testCatalogue(t)
	bank := semantic(items, "FSI – Banking")
	hardware := semantic(items, "Tech - Hardware")
	require.NotNil(t, bank)
	require.NotNil(t, hardware)
	for _, verified := range []*api.Industry{bank, hardware} {
		got, _ := Recognize(items, Employment{Name: "Example", Title: "SAP Consultant"}, verified.Id)
		require.NotNil(t, got)
		assert.Equal(t, verified.Id, got.Id)
	}
	got, _ := Recognize(items, Employment{Name: "Unknown Sdn Bhd", Title: "SAP Consultant"}, "")
	assert.Nil(t, got)
	it := semantic(items, "Information Technology & Services")
	got, _ = Recognize(items, Employment{Name: "ERP services", Title: "SAP Consultant"}, it.Id)
	require.NotNil(t, got)
	assert.Equal(t, "Tech – ERP/CRM/HCM/Application", got.Label)
	// Renames keep semantics; a retired target is never suggested.
	for i := range items {
		if items[i].Id == bank.Id {
			items[i].Label = "Banking renamed"
		}
	}
	got, _ = Recognize(items, Employment{Name: "HSBC", Title: "SAP"}, "")
	require.NotNil(t, got)
	assert.Equal(t, "Banking renamed", got.Label)
}
func TestEmployerSelectionUsesDatesAndAllowsHistoricalGaps(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		jobs []Employment
		want int
	}{
		{"current plus old undated", []Employment{{Name: "Old"}, {Name: "Bank", Current: true, Start: "2024"}}, 1},
		{"latest ended not first", []Employment{{Name: "Old", End: "2019"}, {Name: "Recent", End: "2025-06"}}, 1},
		{"undated with ended needs review", []Employment{{Name: "Unknown"}, {Name: "Ended", End: "2025"}}, -1},
		{"concurrent", []Employment{{Name: "One", Current: true}, {Name: "Two", Current: true}}, -1},
		{"tied end", []Employment{{Name: "One", End: "2025"}, {Name: "Two", End: "2025"}}, -1},
		{"reversed dates", []Employment{{Name: "One", Start: "2025", End: "2020", Current: true}}, -1},
		{"future start", []Employment{{Name: "One", Start: "2028", Current: true}}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { got, _ := SelectEmployer(c.jobs, now); assert.Equal(t, c.want, got) })
	}
	_, ok := DateBound("2025-02-30", false)
	assert.False(t, ok)
}
