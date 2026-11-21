package matching

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func weightsOf(terms []SearchTerm) map[string]float32 {
	out := map[string]float32{}
	for _, t := range terms {
		out[t.Text] = t.Weight
	}
	return out
}

func TestPossibleCandidatesAreLookedForWithTheJobsOwnWords(t *testing.T) {
	jd := "We are looking for an experienced Finance Manager with proven experience within an advanced team.\n" +
		"Requirements: strong skills and the ability to work independently. ACCA or CPA qualified.\n" +
		"Lead the statutory audit and statutory audit preparation; MFRS reporting in SAP.\nSalary: RM 12,000"
	got := weightsOf(SearchTerms(Job{Title: "Finance Manager", CompanyName: "Maju Bhd", Description: &jd, AdTitle: ptrs("Senior Finance Manager")}))

	for _, w := range []string{"experience", "experienced", "proven", "within", "advanced", "skills", "ability", "strong", "team", "work", "independently", "requirements", "looking"} {
		assert.NotContains(t, got, w, "%q says nothing about who fits", w)
	}
	assert.Equal(t, float32(titlePhrase), got["finance manager"], "the title, as a phrase, counts most")
	assert.Equal(t, float32(titleWeight), got["finance"])
	assert.Equal(t, float32(acronymWeight), got["acca"], "a qualification")
	assert.Equal(t, float32(acronymWeight), got["sap"])
	assert.Equal(t, float32(phraseWeight), got["statutory audit"], "a term the description repeats")
	assert.NotContains(t, got, "12,000", "never the salary")
	assert.NotContains(t, got, "maju", "nor the client's name")
}

func TestSearchTermsAreFewAndHeaviestFirst(t *testing.T) {
	long := ""
	for _, w := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet", "kilo", "lima",
		"mike", "november", "oscar", "papa", "quebec", "romeo", "sierra", "tango", "uniform", "victor", "whiskey", "xray", "yankee", "zulu",
		"auditing", "banking", "costing", "dealing", "treasury", "payroll"} {
		long += w + ". "
	}
	terms := SearchTerms(Job{Title: "Accountant", Description: &long})
	assert.Len(t, terms, maxSearchTerms)
	assert.Equal(t, "accountant", terms[0].Text)
	for i := 1; i < len(terms); i++ {
		assert.GreaterOrEqual(t, terms[i-1].Weight, terms[i].Weight)
	}
}
