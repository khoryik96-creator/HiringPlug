package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestPublishingAnAd(t *testing.T) {
	today := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	text := "We are hiring."
	ready := AdDetails{Title: "Accountant", Description: &text, Status: "published"}
	assert.Empty(t, PublishProblem(ready, "open", today, true))
	assert.Contains(t, PublishProblem(ready, "on_hold", today, true), "this job is on hold")
	assert.Empty(t, PublishProblem(ready, "on_hold", today, false), "an ad already up stays up while the job is on hold")
	noText := ready
	noText.Description = nil
	assert.Contains(t, PublishProblem(noText, "open", today, false), "Write the ad's text")
	past := today.AddDate(0, 0, -1)
	expired := ready
	expired.ClosesOn = &past
	assert.Contains(t, PublishProblem(expired, "open", today, true), "closing date has passed")
	assert.Empty(t, PublishProblem(AdDetails{Title: "x", Description: &text, ClosesOn: &today}, "open", today, true), "closing today is fine")

	got, errs := NormalizeAd(AdDetails{Title: "  Senior   Accountant ", SalaryText: ptr("  "), Description: ptr("\n- SAP\n- Excel\n")})
	assert.NoError(t, errs.Err())
	assert.Equal(t, "Senior Accountant", got.Title)
	assert.Nil(t, got.SalaryText)
	assert.Equal(t, "- SAP\n- Excel", *got.Description, "line breaks kept")
	assert.Equal(t, "draft", got.Status)
}

func ptr(s string) *string { return &s }
