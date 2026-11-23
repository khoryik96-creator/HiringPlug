package meaning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning"
)

func ptr[T any](v T) *T { return &v }

// The weights are measured by ai/tests/test_meaning_bench.py, which ranks
// with the same numbers: these pin them, so changing one is deliberate.
func TestTheWeightsAreTheOnesTheBenchMeasured(t *testing.T) {
	t.Parallel()
	role := []meaning.RoleSim{{Title: "Recruiter", Field: 0.8, Work: 0.6}}
	e := meaning.Evidence{Roles: role, PassageWhole: 0.5, PassageContext: 0.4, Profile: 0.3, Words: 1}
	cases := []struct {
		name string
		q    meaning.Query
		want float64
	}{
		{"a title", meaning.Query{HasRole: true}, 0.7*0.8 + 0.3*0.3 + 0.06},
		{"a title and more", meaning.Query{HasRole: true, HasContext: true}, 0.45*0.8 + 0.35*0.4 + 0.2*0.3 + 0.06},
		{"work, read as its family", meaning.Query{HasRole: true, Describes: true}, 0.5*0.5 + 0.5*0.8 + 0.06},
		{"work, no family", meaning.Query{Describes: true}, 0.75*0.5 + 0.25*0.3 + 0.06},
		{"a tool or code", meaning.Query{HasRole: true, Exact: true}, 0.7*0.8 + 0.3*0.3 + 0.25},
	}
	for _, c := range cases {
		assert.InDelta(t, c.want, meaning.Score(c.q, e, 2026).Score, 1e-9, c.name)
	}
	job := meaning.Evidence{Roles: role, Profile: 0.3, Needs: []float64{0.9, 0.6}}
	got := meaning.Score(meaning.Query{HasRole: true, Needs: 3}, job, 2026)
	assert.InDelta(t, 0.55*0.8+0.25*(0.9+0.6)/3+0.2*0.3, got.Score, 1e-9, "a need their CV says nothing about counts as unmet")
}

func TestTheNewestTitleAtTheLevelAskedForCountsMost(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 1, meaning.Recency(nil, 2026), 1e-9, "their job now")
	assert.InDelta(t, 0.85, meaning.Recency(ptr(2023), 2026), 1e-9)
	assert.InDelta(t, 0.55, meaning.Recency(ptr(2005), 2026), 1e-9, "never below: it is still their experience")
	assert.InDelta(t, 1, meaning.LevelFit(ptr(5), ptr(5)), 1e-9)
	assert.InDelta(t, 0.94, meaning.LevelFit(ptr(5), ptr(3)), 1e-9)
	assert.InDelta(t, 0.91, meaning.LevelFit(ptr(7), ptr(0)), 1e-9, "at most three levels count")
	assert.InDelta(t, 1, meaning.LevelFit(nil, ptr(3)), 1e-9, "no level asked for")

	// A recruiter ten years ago, a sales director now: the recruiter title counts less.
	q := meaning.Query{HasRole: true, Level: ptr(2)}
	was := meaning.Score(q, meaning.Evidence{Roles: []meaning.RoleSim{
		{Title: "Sales Director", Field: 0.3, Work: 0.3},
		{Title: "Recruitment Consultant", Field: 0.9, Work: 0.9, EndYear: ptr(2013), Level: ptr(2)},
	}}, 2026)
	now := meaning.Score(q, meaning.Evidence{Roles: []meaning.RoleSim{{Title: "Recruiter", Field: 0.9, Work: 0.9, Level: ptr(2)}}}, 2026)
	assert.Equal(t, "Recruitment Consultant", was.Role.Title, "why: the title that matched")
	assert.Less(t, was.Score, now.Score)
}

// An Account Executive who sells is not an Accounts Executive, however close
// the words: a best title in another field (neither the one asked for nor a
// neighbour of it) counts for less; a neighbour or an unknown field does not.
func TestABestTitleInAnotherFieldCountsForLess(t *testing.T) {
	t.Parallel()
	q := meaning.Query{HasRole: true, Family: "accounting", Near: []string{"finance management", "audit"}}
	in := func(family *string) meaning.Evidence {
		return meaning.Evidence{Roles: []meaning.RoleSim{{Title: "Account Executive", Family: family, Field: 0.7, Work: 0.5}}, Profile: 0.5}
	}
	same := meaning.Score(q, in(ptr("accounting")), 2026)
	assert.InDelta(t, 0.7*0.7+0.3*0.5, same.Score, 1e-9)
	assert.Equal(t, "good", meaning.Strength(same.Score))
	assert.False(t, same.AnotherField)
	other := meaning.Score(q, in(ptr("sales")), 2026)
	assert.InDelta(t, 0.75*same.Score, other.Score, 1e-9)
	assert.True(t, other.AnotherField)
	assert.Equal(t, "possible", meaning.Strength(other.Score), "a good match by its words, possible in another field")
	for _, kept := range []*string{ptr("finance management"), ptr(""), nil} {
		assert.InDelta(t, same.Score, meaning.Score(q, in(kept), 2026).Score, 1e-9, "a neighbour, or a field not known")
	}
	assert.InDelta(t, same.Score, meaning.Score(meaning.Query{HasRole: true}, in(ptr("sales")), 2026).Score, 1e-9,
		"a search with no field asks for none")

	// Their CV naming a tool asked for still counts in full.
	exact := q
	exact.Exact = true
	withWords := in(ptr("sales"))
	withWords.Words = 1
	assert.InDelta(t, other.Score+0.25, meaning.Score(exact, withWords, 2026).Score, 1e-9)

	// Their own title in the field asked for wins over a newer one outside it.
	moved := meaning.Evidence{Roles: []meaning.RoleSim{
		{Title: "Sales Director", Family: ptr("sales"), Field: 0.45, Work: 0.45},
		{Title: "Accountant", Family: ptr("accounting"), Field: 0.9, Work: 0.9, EndYear: ptr(2022)},
	}}
	got := meaning.Score(q, moved, 2026)
	assert.Equal(t, "Accountant", got.Role.Title)
	assert.False(t, got.AnotherField)
}

func TestWhyAndStrengthAreSaidInWords(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "strong", meaning.Strength(0.7))
	assert.Equal(t, "good", meaning.Strength(0.55))
	assert.Equal(t, "possible", meaning.Strength(0.4))
	assert.Equal(t, "now", meaning.When(nil))
	assert.Equal(t, "until 2019", meaning.When(ptr(2019)))
	assert.Equal(t, []string{"SAP", "S/4HANA"}, meaning.WordsIn("led [[SAP]] [[S/4HANA]] go-live and [[sap]] support"))
}

func TestASearchesOwnWordsLeaveOutWhatEverySearchSays(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"headhunter", "finance"}, meaning.SearchWords("senior headhunter for finance roles"))
	assert.Equal(t, []string{"S/4HANA", "C++", "ISO", "9001"}, meaning.SearchWords("S/4HANA, C++ and ISO 9001"))
	assert.Empty(t, meaning.SearchWords("someone who has experience"))
}

func TestTheSameWordsAreReadOnceAndAChangedCandidateAgain(t *testing.T) {
	t.Parallel()
	const model = "all-MiniLM-L6-v2@1110a243/r3"
	assert.Equal(t, meaning.QueryHash(model, "", "Talent  Acquisition "), meaning.QueryHash(model, "", "talent acquisition"))
	assert.NotEqual(t, meaning.QueryHash(model, "", "recruiter"), meaning.QueryHash(model, "Recruiter", ""), "a job is not a search")
	assert.NotEqual(t, meaning.QueryHash(model, "", "recruiter"), meaning.QueryHash("all-MiniLM-L6-v2@1110a243/r4", "", "recruiter"),
		"new reading rules read the words again")

	src := meaning.Source{CandidateID: "cand_1", CurrentTitle: "Accountant", CV: &meaning.CV{DocumentID: "doc_1", Text: "Full set of accounts"}}
	same := src
	assert.Equal(t, meaning.Fingerprint(src), meaning.Fingerprint(same))
	newCV := src
	newCV.CV = &meaning.CV{DocumentID: "doc_2", Text: "Full set of accounts"}
	edited := src
	edited.CV = &meaning.CV{DocumentID: "doc_1", Text: "Full set of accounts and audits"}
	promoted := src
	promoted.Work = []meaning.Work{{Title: "Senior Accountant", Current: true}}
	for _, changed := range []meaning.Source{newCV, edited, promoted} {
		assert.NotEqual(t, meaning.Fingerprint(src), meaning.Fingerprint(changed))
	}
}
