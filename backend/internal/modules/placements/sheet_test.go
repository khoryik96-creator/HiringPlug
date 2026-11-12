package placements

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSheet(t *testing.T) {
	for workType, want := range map[string]string{"contract": Contract, "temporary": Contract, "full_time": Permanent, "internship": Permanent} {
		assert.Equal(t, want, JobTypeOf(&workType), workType)
	}
	assert.Equal(t, Permanent, JobTypeOf(nil), "no work type: permanent")

	app := Application{JobTitle: "Finance Manager", CompanyName: "Maju Foods", WorkType: ptr("contract")}
	s := Sheet{}.WithJob(app)
	assert.Equal(t, "Finance Manager", *s.JobTitle)
	assert.Equal(t, "Maju Foods", *s.CompanyName)
	assert.Equal(t, Contract, s.JobType)
	s = Sheet{JobTitle: ptr("Finance Lead"), JobType: Permanent}.WithJob(app)
	assert.Equal(t, "Finance Lead", *s.JobTitle, "what was entered stays")
	assert.Equal(t, Permanent, s.JobType)
	assert.Equal(t, "Finance Lead", OnSheet(app, s).JobTitle)
	assert.Equal(t, "Maju Foods", OnSheet(app, Sheet{}).CompanyName, "nothing on the sheet: the job's")

	d, errs, _ := Normalize(Details{StartDate: day("2026-12-01"), Sheet: Sheet{PONumber: ptr("  "), EndDate: ptr(day("2026-11-30"))}}, day("2026-11-01"))
	assert.Nil(t, d.PONumber, "blank: none")
	assert.Equal(t, Permanent, d.JobType)
	assert.Error(t, errs.Err(), "an end before the start")
	_, errs, _ = Normalize(Details{StartDate: day("2026-12-01"), Sheet: Sheet{JobType: "temporary"}}, day("2026-11-01"))
	assert.Error(t, errs.Err())
}
