package cvformats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// CV Studio's formatting rules (cvstudio_cv_normalize.py, generate.js).
func TestDutiesTakeOutTheSourcesMarkers(t *testing.T) {
	assert.Equal(t, "Received calls\nProvide support\nReviewed claims\nFiled reports\nAudited 5 branches\nLed audits\nTrained staff\nWrote SOPs",
		Duties("-Received calls\n• Provide support\n(i)Reviewed claims\n1. Filed reports\n2)Audited 5 branches\na. led audits\nii. Trained staff\n•  - Wrote SOPs"))
	assert.Equal(t, "-5% variance on budget\n3.5 years of audits\n5-star service", Duties("-5% variance on budget\n3.5 years of audits\n5-star service"),
		"figures are not markers")
	assert.Equal(t, "Closing\nReporting", Duties("Closing\n-\n--\n•\n\nReporting\nReporting"), "marker-only lines and repeats go")
	assert.Equal(t, "Led the team", Duties("Position: Retrieved Resumes (SiVA folder: Finance); Date Applied: 1 Nov 2026\nLed the team"),
		"job-board tracking lines are not part of a CV")
}

func TestSmartCase(t *testing.T) {
	assert.Equal(t, "Lenovo PCCW Solutions Sdn Bhd", SmartCase("LENOVO PCCW SOLUTIONS SDN BHD", true))
	assert.Equal(t, "TDCX Malaysia Sdn Bhd", SmartCase("TDCX MALAYSIA SDN BHD", true), "vowelless brands stay in capitals")
	assert.Equal(t, "iFAST Capital Sdn Bhd", SmartCase("IFAST CAPITAL SDN BHD", true))
	assert.Equal(t, "Virtual Calibre MSC Sdn Bhd", SmartCase("Virtual Calibre MSC SDN BHD", true), "mixed case: only suffixes")
	assert.Equal(t, "McKinsey & Company", SmartCase("McKinsey & Company", true), "brand casing kept")
	assert.Equal(t, "Balrath Outsourcing Service", SmartCase("Balrath outsourcing Service", true), "a stray lower-case company word")
	assert.Equal(t, "Senior Manager of Finance (APAC)", SmartCase("SENIOR MANAGER OF FINANCE (APAC)", false))
	assert.Equal(t, "Sr. SAP FICO Consultant", SmartCase("SR. SAP FICO CONSULTANT", false))
	assert.Equal(t, "Head of HR, KL & Penang", SmartCase("HEAD OF HR, KL & PENANG", false))
	assert.Equal(t, "Software engineer", SmartCase("Software engineer", false), "a title in mixed case is kept")
}

func TestPlaceholderDegreesAndOneYearDates(t *testing.T) {
	assert.Equal(t, "", CleanQualification("No Degree"))
	assert.Equal(t, "", CleanQualification("N/A"))
	assert.Equal(t, "Diploma in IT", CleanQualification("No Degree: Diploma in IT"))
	assert.Equal(t, "Bachelor of Arts", CleanQualification("Bachelor of Arts"))
	y := "2018"
	assert.Equal(t, "2018", Dates(Job{StartDate: &y, EndDate: &y}))
}
