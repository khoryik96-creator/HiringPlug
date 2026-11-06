package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFieldsFillFromTheirTokensAndPlaceholders(t *testing.T) {
	values := map[string]string{
		"candidate.firstName": "Siti", "candidate.expectedSalary": "RM 12,000 per month", "job.title": "Auditor <Senior>",
	}
	f := FillFields("Dear [first NAME], for [Job title] {{candidate.firstName}} expects [Expected salary]; [Notice period] [optional] [x]", values, false)
	assert.Equal(t, "Dear Siti, for Auditor <Senior> Siti expects RM 12,000 per month; [Notice period] [optional] [x]", f.Text,
		"case does not matter; text in brackets that names no field is left alone")
	assert.Equal(t, []string{"[Notice period]"}, f.Missing)
	assert.True(t, f.Salary)

	h := FillFields("<p>[Job title]</p>", values, true)
	assert.Equal(t, "<p>Auditor &lt;Senior&gt;</p>", h.Text, "values are escaped in HTML")
	assert.False(t, h.Salary)
	assert.Contains(t, FillFields("{{interview.when}}", values, false).Missing, "[Interview time]")
}

func TestFormattedMessagesKeepOnlyWhatTheToolbarMakes(t *testing.T) {
	banner := "ebn_01jbz6w0m0k8q9x2r3s4t5v6w7"
	in := `<p style="text-align:center;color:#0055aa" onclick="x()">Hi <b>Ms Tan</b><script>alert(1)</script></p>` +
		`<img src="https://tracker.example/p.gif"><img data-banner-id="` + banner + `" src="blob:http://app/1" alt="banner">` +
		`<a href="javascript:alert(1)">bad</a> <a href="https://hire.example/job">job</a><iframe src="x"></iframe>`
	out := SanitizeHTML(in)
	assert.NotContains(t, out, "script")
	assert.NotContains(t, out, "onclick")
	assert.NotContains(t, out, "tracker.example", "no image loads from elsewhere")
	assert.NotContains(t, out, "blob:")
	assert.NotContains(t, out, "javascript")
	assert.NotContains(t, out, "iframe")
	assert.Contains(t, out, `text-align: center`)
	assert.Contains(t, out, `data-banner-id="`+banner+`"`)
	assert.Contains(t, out, `href="https://hire.example/job"`)
	assert.Equal(t, []string{banner}, BannerIDs(out+out))
	assert.Contains(t, WithBannerCIDs(out), `src="cid:`+banner+`"`)
}

func TestThePlainTextVersion(t *testing.T) {
	text := TextFromHTML(`<p>Dear <b>Ms</b> Tan,</p><p>Please find:</p><ul><li>Siti's CV</li><li>Her notice: 1 month</li></ul>` +
		`<ol><li>First</li><li>Second</li></ol><div>See <a href="https://hire.example/j">the job</a><br>Thanks</div>`)
	assert.Equal(t, "Dear Ms Tan,\n\nPlease find:\n\n- Siti's CV\n- Her notice: 1 month\n\n1. First\n2. Second\n\nSee the job (https://hire.example/j)\nThanks", text)
}

func TestHowFieldsAreWritten(t *testing.T) {
	maximum := "14000.00"
	assert.Equal(t, "RM 9,500 per month", FormatSalary("9500.00", "MYR", "month"))
	assert.Equal(t, "SGD 120,000.50 per year", FormatSalary("120000.50", "SGD", "year"))
	assert.Equal(t, "RM 12,000 – 14,000 per month", FormatRange("12000.00", &maximum, "MYR", "month"))
	assert.Equal(t, "Immediate", FormatNotice(0, "month"))
	assert.Equal(t, "2 weeks", FormatNotice(2, "week"))
	rpt, pr := "rp_t", "permanent_resident"
	assert.Equal(t, "Expat (RP-T)", FormatWorkAuthorization("expat_eligible", &rpt))
	assert.Equal(t, "Permanent resident", FormatWorkAuthorization("expat_eligible", &pr))
	assert.Equal(t, "Needs an employment pass", FormatWorkAuthorization("requires_visa", nil))
}
