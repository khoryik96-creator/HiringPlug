package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var jdToday = time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)

func TestReadJD(t *testing.T) {
	t.Run("labelled details, bullets as lines, labels left out of the text", func(t *testing.T) {
		d := ReadJD(`Job Description

Position: Finance Manager
Location : Petaling Jaya, Selangor
Salary: RM 12,000 - 15,000 per month
Employment Type: Permanent, Full-time
Work arrangement: Hybrid (3 days in office)
Closing date: 31/12/2026

About the role
Maju Foods Berhad is looking for a Finance Manager to lead its finance team.

Responsibilities
•  Lead month-end closing and reporting
●	Manage a team of 5 accountants
1. Liaise with external auditors

Requirements
- ACCA / CPA
- 8 years in FMCG finance`, jdToday)
		assert.Equal(t, "Finance Manager", d.Title)
		assert.Equal(t, "Petaling Jaya, Selangor", d.Location)
		assert.Equal(t, "MY", d.Country)
		assert.Equal(t, "RM 12,000 - 15,000 per month", d.SalaryText)
		assert.Equal(t, "full_time", d.WorkType)
		assert.Equal(t, "hybrid", d.WorkArrangement)
		assert.Equal(t, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), *d.ClosesOn)
		assert.Equal(t, `Job Description

About the role
Maju Foods Berhad is looking for a Finance Manager to lead its finance team.

Responsibilities
- Lead month-end closing and reporting
- Manage a team of 5 accountants
- Liaise with external auditors

Requirements
- ACCA / CPA
- 8 years in FMCG finance`, d.Description)
	})

	t.Run("no labels: the first short line is the title, the rest is found in the text", func(t *testing.T) {
		d := ReadJD(`Senior Data Engineer

We are a fintech in Singapore. This is a 12-month contract role, fully remote.
Pay: S$8,000/month. Apply by 15 January 2027.`, jdToday)
		assert.Equal(t, "Senior Data Engineer", d.Title)
		assert.Equal(t, "Singapore", d.Location)
		assert.Equal(t, "SG", d.Country)
		assert.Equal(t, "contract", d.WorkType)
		assert.Equal(t, "remote", d.WorkArrangement)
		assert.Equal(t, "S$8,000/month", d.SalaryText)
		assert.Nil(t, d.ClosesOn, "a date in a sentence is not a closing date")
		assert.NotContains(t, d.Description, "Senior Data Engineer")
	})

	t.Run("a closing date already past, or not a real date, is dropped", func(t *testing.T) {
		assert.Nil(t, ReadJD("Auditor\nDeadline: 1 Jan 2025", jdToday).ClosesOn)
		assert.Nil(t, ReadJD("Auditor\nDeadline: 31/02/2027", jdToday).ClosesOn)
		assert.Equal(t, time.Date(2027, 6, 2, 0, 0, 0, 0, time.UTC), *ReadJD("Auditor\nApply by: June 2, 2027", jdToday).ClosesOn)
		assert.Equal(t, time.Date(2027, 3, 4, 0, 0, 0, 0, time.UTC), *ReadJD("Auditor\nClosing: 2027-03-04", jdToday).ClosesOn)
	})

	t.Run("a heading first is not the title", func(t *testing.T) {
		assert.Equal(t, "", ReadJD("Responsibilities\n- Audit accounts", jdToday).Title)
	})
}

func TestReadJDUndoesPDFLineWrapping(t *testing.T) {
	d := ReadJD(`Finance Manager
About Maju Foods Berhad
Maju Foods Berhad is one of Malaysia's leading FMCG manufacturers, with brands
in over 20 countries. We are looking for a Finance Manager.
Key Responsibilities
• Lead month-end and year-end closing, and management reporting
to the CFO
• Manage a team of 5
Requirements
• ACCA, CPA or CIMA qualified`, jdToday)
	assert.Equal(t, `About Maju Foods Berhad
Maju Foods Berhad is one of Malaysia's leading FMCG manufacturers, with brands in over 20 countries. We are looking for a Finance Manager.

Key Responsibilities
- Lead month-end and year-end closing, and management reporting to the CFO
- Manage a team of 5

Requirements
- ACCA, CPA or CIMA qualified`, d.Description)
}
