package assistant

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func allText(sections []Section) string {
	var b strings.Builder
	for _, s := range sections {
		b.WriteString(s.Title + "\n" + s.Text + "\n")
	}
	return b.String()
}

func TestTheAISeesNoContactDetailsOrName(t *testing.T) {
	c := Candidate{
		ID: "cand_1", Name: "Siti Aisyah binti Ahmad", Email: "siti.aisyah@mail.com", Phone: "+60 12-345 6789",
		CurrentTitle: ptr("Senior Accountant"), CurrentCompany: ptr("Contoh Sdn Bhd"), Location: ptr("Petaling Jaya"),
		WorkAuthorization: ptr("local (no work pass needed)"), NoticePeriod: ptr("1 month(s)"), Skills: []string{"SAP", "Excel"},
		Work: []Work{
			{Title: ptr("Senior Accountant"), Company: ptr("Contoh Sdn Bhd"), StartDate: ptr("2021-03"), Current: true,
				Description: ptr("Siti led month-end close. Reach her at siti.aisyah@mail.com.")},
			{Title: ptr("Accountant"), StartDate: ptr("2017"), EndDate: ptr("2021")},
		},
		Education: []Qualification{{Qualification: ptr("Bachelor of Accounting"), Institution: ptr("UiTM"), EndYear: ptr(2016)}},
	}
	cv := "SITI AISYAH BINTI AHMAD\nsiti.aisyah@mail.com | 012-345 6789 | linkedin.com/in/sitiaisyah\nAccountant 2017-2021"
	got := allText(CandidateSections(c, cv))

	for _, leak := range []string{"Siti", "SITI", "Aisyah", "AISYAH", "Ahmad", "siti.aisyah@mail.com", "345 6789", "linkedin.com"} {
		assert.NotContains(t, got, leak)
	}
	assert.Contains(t, got, "Current title: Senior Accountant")
	assert.Contains(t, got, "Right to work: local (no work pass needed)")
	assert.Contains(t, got, "- Senior Accountant at Contoh Sdn Bhd (2021-03 to now)")
	assert.Contains(t, got, "the candidate led month-end close")
	assert.Contains(t, got, "- Accountant at employer not given (2017 to 2021)")
	assert.Contains(t, got, "- Bachelor of Accounting, UiTM (2016)")
	assert.Contains(t, got, "Accountant 2017-2021", "years are not phone numbers")
}

func TestSalaryFiguresAreTakenOut(t *testing.T) {
	for in, want := range map[string]string{
		"Expected salary: RM 5,000 negotiable":                      "Expected [salary removed]",
		"Accountant, 2019-2023. Last drawn pay RM8.5k":              "Accountant, 2019-2023. [salary removed]",
		"Gaji semasa: 4500":                                         "[salary removed]",
		"Current package / CTC: MYR 120,000 a year":                 "Current package / [salary removed]",
		"Managed compensation and benefits for 300 staff":           "Managed compensation and benefits for 300 staff",
		"Processed salary for staff, cut errors by half since 2019": "Processed salary for staff, cut errors by half since 2019",
		"Expected salary: 2000":                                     "Expected [salary removed]",
		"Saved RM 1.2 million on audit fees":                        "Saved RM 1.2 million on audit fees",
	} {
		assert.Equal(t, want, WithoutSalary(in), in)
	}
	got := allText(JobSections(Job{Title: "HR", Description: ptr("Run payroll.\nSalary: RM 9,999 - 12,000\nBenefits: medical")}))
	assert.Contains(t, got, "Run payroll.\n[salary removed]\nBenefits: medical")
}

func TestLongTextIsCut(t *testing.T) {
	got := CandidateSections(Candidate{Name: "X"}, strings.Repeat("a", maxCVText+50))
	require.Len(t, got, 2)
	assert.Equal(t, "(nothing recorded)", got[0].Text)
	assert.True(t, strings.HasSuffix(got[1].Text, "[... cut here]"))
	assert.Len(t, []rune(got[1].Text), maxCVText+len("\n[... cut here]"))
}

func TestJobAndApplicationSections(t *testing.T) {
	j := Job{Title: "Finance Manager", CompanyName: "Maju Bhd", Status: "open", Openings: 2, Location: ptr("KL"), Description: ptr("Lead a team of 5.")}
	got := allText(ApplicationSections(Application{Stage: "interview"}, Candidate{Name: "Ali"}, "", j))
	assert.Contains(t, got, "Title: Finance Manager\nEmployer: Maju Bhd\nStatus: open\nOpenings: 2\nLocation: KL")
	assert.Contains(t, got, "Job description\nLead a team of 5.")
	assert.Contains(t, got, "Stage: interview")
	assert.NotContains(t, got, "CV text", "no CV, no CV section")
}

func TestQuestionRules(t *testing.T) {
	ok, err := CheckQuestion(Question{SubjectType: "candidate", SubjectID: "cand_1", Kind: "question", Question: ptr("  Why did she leave?  ")})
	require.NoError(t, err)
	assert.Equal(t, "Why did she leave?", *ok.Question)

	for name, q := range map[string]Question{
		"a question needs words":       {SubjectType: "job", SubjectID: "job_1", Kind: "question", Question: ptr("   ")},
		"a summary takes no question":  {SubjectType: "job", SubjectID: "job_1", Kind: "summary", Question: ptr("why")},
		"fit is for applications only": {SubjectType: "candidate", SubjectID: "cand_1", Kind: "fit"},
		"a subject is needed":          {SubjectType: "candidate", SubjectID: " ", Kind: "summary"},
	} {
		_, err := CheckQuestion(q)
		assert.Error(t, err, name)
	}
}

func TestMonthStart(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	// 31 Oct 2026, 17:00 UTC is already 1 Nov in Kuala Lumpur.
	got := MonthStart(time.Date(2026, 10, 31, 17, 0, 0, 0, time.UTC), kl)
	assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, kl), got)
}
