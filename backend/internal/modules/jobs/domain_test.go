package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(s string) *string { return &s }

func TestNormalizeFillsDefaults(t *testing.T) {
	d, errs := Normalize(Details{Title: "  Senior   Accountant ", Location: ptr("  "), Description: ptr("\n Duties \n")})
	require.NoError(t, errs.Err())
	assert.Equal(t, "Senior Accountant", d.Title)
	assert.Equal(t, "open", d.Status)
	assert.Equal(t, 1, d.Openings)
	assert.Nil(t, d.Location)
	assert.Equal(t, "Duties", *d.Description)

	_, errs = Normalize(Details{Title: " "})
	require.Len(t, errs, 1)
	assert.Equal(t, "title", errs[0].Field)
}

func TestValidateSalary(t *testing.T) {
	assert.Empty(t, ValidateSalary(nil))
	assert.Empty(t, ValidateSalary(&SalaryRange{Min: "8000", Max: ptr("8000.00"), Currency: "MYR", Period: "month"}))
	assert.Empty(t, ValidateSalary(&SalaryRange{Min: "8000", Currency: "MYR", Period: "month"}))
	errs := ValidateSalary(&SalaryRange{Min: "9000.50", Max: ptr("9000.4"), Currency: "MYR", Period: "month"})
	require.Len(t, errs, 1)
	assert.Equal(t, "range.max", errs[0].Field)
}

func TestPatchOnlyChangesWhatIsSent(t *testing.T) {
	before := Details{CompanyID: "comp_a", Title: "Auditor", Status: "open", Openings: 2, Location: ptr("KL")}
	p := Patch{}
	p.Status.Set, p.Status.Value = true, "on_hold"
	p.Location.Set = true
	after := p.Apply(before)
	assert.Equal(t, "on_hold", after.Status)
	assert.Nil(t, after.Location)
	assert.Equal(t, 2, after.Openings)
	assert.Equal(t, "Auditor", after.Title)
}
