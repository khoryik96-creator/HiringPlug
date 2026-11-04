package matching

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrs(s string) *string { return &s }

const (
	jobText = "Job title: Finance Manager\n\nJob description:\n- Degree in Accounting or Finance\n- At least 5 years in audit\n" +
		"- Proficient in SAP and Excel\n- Fluent in Mandarin\n- Malaysians only, aged below 35"
	cvText = "Senior Accountant, Contoh Sdn Bhd, 2019 – now\nBachelor of Accounting, UiTM\nAuditor at KPMG, 2016-2019\nSkills: Excel, SAP FICO"
)

func TestTheJobTextHasItsDetailsDescriptionAndAdButNoSalary(t *testing.T) {
	text, ok := JobText(Job{
		Title: "Finance Manager", CompanyName: "Maju Bhd", Location: ptrs("Kuala Lumpur"), WorkArrangement: ptrs("hybrid"),
		Description: ptrs("Lead the finance team.\nSalary: RM 9,999 a month"), AdTitle: ptrs("Finance Manager (KL)"), AdText: ptrs("- CPA or ACCA\nWe offer RM 12,000"),
	})
	assert.True(t, ok)
	assert.Equal(t, "Job title: Finance Manager\nEmployer: Maju Bhd\nLocation: Kuala Lumpur\nArrangement: hybrid\n\n"+
		"Job description:\nLead the finance team.\n[salary removed]\n\nJob ad (Finance Manager (KL)):\n- CPA or ACCA\nWe offer RM 12,000", text,
		"the ad's own salary line has no salary word, so it stays: ads are public")

	_, ok = JobText(Job{Title: "Finance Manager", Description: ptrs("  ")})
	assert.False(t, ok, "nothing to compare without a description or an ad")
	text, ok = JobText(Job{Title: "Clerk", AdText: ptrs("Filing.")})
	assert.True(t, ok)
	assert.Equal(t, "Job title: Clerk\n\nJob ad:\nFiling.", text)
}

func TestTheCVTextHasNoNameContactsOrSalary(t *testing.T) {
	got := CVText(Candidate{Name: "Siti Aisyah", Email: "siti@mail.com", Phone: "012-345 6789"},
		"SITI AISYAH\nsiti@mail.com | 012-345 6789\nAccountant at Contoh, 2019-2023\nExpected salary: RM 8,000")
	assert.Equal(t, "the candidate the candidate\n[email removed] | [phone removed]\nAccountant at Contoh, 2019-2023\nExpected [salary removed]", got)
	assert.Empty(t, CVText(Candidate{Name: "X"}, " \n "))
	assert.True(t, strings.HasSuffix(CVText(Candidate{}, strings.Repeat("a", maxCVText+1)), "[... cut here]"))
}

func TestRequirementsMustQuoteTheJobAndEvidenceTheCV(t *testing.T) {
	reasons, dropped := Check(jobText, cvText, []Claim{
		{Requirement: "Degree in Accounting or Finance", Verdict: Met, Evidence: ptrs("Bachelor of Accounting, UiTM"), Note: ptrs("Accounting degree.")},
		{Requirement: "At least 5 years in audit.", Verdict: Partly, Evidence: ptrs("Auditor at KPMG, 2016-2019"), Note: ptrs("3 years of audit.")},
		{Requirement: "Proficient in SAP and Excel", Verdict: Met, Evidence: ptrs("Skills: Excel, SAP S/4HANA")},
		{Requirement: "Fluent in Mandarin", Verdict: Met},
		{Requirement: "CPA qualification", Verdict: Met, Evidence: ptrs("Bachelor of Accounting")},
		{Requirement: "degree in accounting   or finance", Verdict: Missing},
		{Requirement: "Malaysians only, aged below 35", Verdict: Met, Evidence: ptrs("Senior Accountant")},
		{Requirement: "Proficient in SAP and Excel", Verdict: "unknown"},
	})
	assert.Equal(t, 2, dropped, "CPA is not in the job's text; an unknown verdict cannot be scored")
	require.Len(t, reasons, 4, "the same requirement once; none about who a person is")
	assert.Equal(t, Reason{Requirement: "Degree in Accounting or Finance", Verdict: Met, Evidence: ptrs("Bachelor of Accounting, UiTM"), Note: ptrs("Accounting degree.")}, reasons[0])
	assert.Equal(t, Partly, reasons[1].Verdict, "punctuation at the end is not part of the quote")
	assert.Equal(t, Reason{Requirement: "Proficient in SAP and Excel", Verdict: Missing, Note: ptrs(noteBadEvidence)}, reasons[2],
		"evidence that is not in the CV does not count")
	assert.Equal(t, Reason{Requirement: "Fluent in Mandarin", Verdict: Missing, Note: ptrs(noteNoEvidence)}, reasons[3])

	score, ok := Score(reasons)
	require.True(t, ok)
	assert.Equal(t, 38, score, "(1 + 0.5) of 4 requirements")
}

func TestOnlyFifteenRequirementsAndEvidenceOnMissingOnesIsChecked(t *testing.T) {
	var job strings.Builder
	var claims []Claim
	for i := range 20 {
		fmt.Fprintf(&job, "- Skill number %d\n", i)
		claims = append(claims, Claim{Requirement: fmt.Sprintf("Skill number %d", i), Verdict: Missing, Evidence: ptrs("not in the CV")})
	}
	reasons, dropped := Check(job.String(), cvText, claims)
	assert.Zero(t, dropped)
	assert.Len(t, reasons, maxRequirements)
	assert.Nil(t, reasons[0].Evidence, "evidence must be CV text, whatever the verdict")
	assert.Nil(t, reasons[0].Note)
	score, _ := Score(reasons)
	assert.Zero(t, score)
}

func TestRequirementsAboutWhoAPersonIsAreNeverScored(t *testing.T) {
	for _, req := range []string{"Malaysians only", "Aged between 25 and 35", "Age: 30 and below", "Female candidates preferred",
		"Bumiputera", "Muslim", "Single status", "Permanent resident or citizen", "Not more than 40 years old"} {
		assert.Regexp(t, personal, req)
	}
	for _, req := range []string{"Health and safety certificate", "Debugging race conditions", "Manage 5 people", "Agency experience",
		"Fluent in Mandarin and Malay", "Single point of contact for clients", "Stakeholder management"} {
		assert.NotRegexp(t, personal, req, "a skill, not who a person is")
	}
}

func TestScoreAndWhatOthersSee(t *testing.T) {
	_, ok := Score(nil)
	assert.False(t, ok, "no requirements, no score")
	all := []Reason{{Verdict: Met}, {Verdict: Met}, {Verdict: Partly}}
	score, _ := Score(all)
	assert.Equal(t, 83, score)

	reasons := []Reason{{Requirement: "SAP", Verdict: Met, Evidence: ptrs("SAP FICO")}}
	assert.Equal(t, reasons, Visible(reasons, true))
	assert.Nil(t, Visible(reasons, false)[0].Evidence, "the evidence quotes the CV")
	assert.NotNil(t, reasons[0].Evidence, "the original is untouched")

	assert.Nil(t, Summary(ptrs("  ")))
	long := Summary(ptrs(strings.Repeat("a", maxSummary+10)))
	assert.Len(t, []rune(*long), maxSummary)
	assert.True(t, strings.HasSuffix(*long, "…"))
}
