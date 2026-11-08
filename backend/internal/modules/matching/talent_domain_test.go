package matching

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func TestTheAIsRankingIsCheckedAgainstWhoItWasSent(t *testing.T) {
	sent := []string{"cand_a", "cand_b", "cand_c"}
	got := TalentResults(sent, []api.TalentRanked{
		{Ref: "C2", Fit: 140, Verdict: "strong", Reasons: []string{"  SAP BW   upgrades "}, Gaps: []string{}},
		{Ref: "c1", Fit: 40, Verdict: "nonsense", Reasons: []string{""}, Gaps: []string{"No BPC"}},
		{Ref: "C2", Fit: 10, Verdict: "weak"},   // repeated
		{Ref: "C9", Fit: 99, Verdict: "strong"}, // never sent
		{Ref: "Siti", Fit: 99, Verdict: "strong"},
	})
	assert.Equal(t, []TalentResult{
		{CandidateID: "cand_b", Fit: 100, Verdict: "strong", Reasons: []string{"SAP BW upgrades"}, Gaps: []string{}},
		{CandidateID: "cand_a", Fit: 40, Verdict: "weak", Reasons: []string{}, Gaps: []string{"No BPC"}},
	}, got, "best first; fit within 0-100; a verdict the AI did not give follows the fit")
}

func TestTheJobDescriptionOrdersThePool(t *testing.T) {
	pool := []PoolCandidate{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	got := TalentOrder(pool, []string{"c", "x", "a"})
	var ids []string
	for _, c := range got {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"c", "a", "b", "d"}, ids, "CVs sharing words first, then the rest in the search's order")
}

func TestWithoutAJobDescriptionTheAIIsToldTheSearchButNoSalary(t *testing.T) {
	b, loc, notice := "SAP AND BW", "Shah Alam", 30
	skills := []string{"SAP BW", "ABAP"}
	got := TalentRequirements("", api.CandidateSearch{Boolean: &b, Skills: &skills, Location: &loc, NoticeMaxDays: &notice,
		ExpectedSalaryMax: &api.SalaryCap{Amount: "12000", Currency: "MYR"}})
	assert.Contains(t, got, "Boolean search: SAP AND BW")
	assert.Contains(t, got, "Skills: SAP BW, ABAP")
	assert.Contains(t, got, "at most 30 days")
	assert.NotContains(t, got, "12000")
	assert.Equal(t, "Senior SAP consultant", TalentRequirements("  Senior SAP consultant ", api.CandidateSearch{Boolean: &b}))
}
