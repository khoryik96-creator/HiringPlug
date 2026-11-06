package market

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/simpledoc"
)

func TestTitleWords(t *testing.T) {
	assert.Equal(t, "finance", TitleWords("Senior Finance Manager"))
	assert.Equal(t, "data engineer contract", TitleWords("Lead Data Engineer (Contract)"))
	assert.Equal(t, "c# developer", TitleWords("Senior C# Developer"))
	assert.Equal(t, "manager", TitleWords("Manager"), "only level words: all of them")
	assert.Equal(t, "accountant", TitleWords("Assistant Accountant II"))
}

func TestBuild(t *testing.T) {
	month := 1
	p := Build([]Member{
		{Company: "Nestlé", Availability: "looking", WorkAuth: "local", NoticeValue: &month, NoticeUnit: "month", Location: "Kuala Lumpur", Country: "MY"},
		{Company: "Nestlé", WorkAuth: "expat_eligible", WorkBasis: "permanent_resident", Country: "SG"},
		{Company: " ", WorkAuth: "requires_visa"},
	}, []Pay{
		{CurrentAmount: "10000", CurrentCurrency: "MYR", CurrentPeriod: "month"},
		{CurrentAmount: "8000", CurrentCurrency: "SGD", CurrentPeriod: "month"},
		{CurrentAmount: "", ExpectedAmount: "12000.50", ExpectedCurrency: "MYR", ExpectedPeriod: "month"},
	})
	assert.Equal(t, 3, p.Total)
	assert.Equal(t, []Count{{"Nestlé", 2}}, p.Companies, "blank companies are not counted")
	assert.Equal(t, []Count{{"unknown", 2}, {"looking", 1}}, p.Availability)
	assert.Equal(t, []Count{{"local", 1}, {"needs_pass", 1}, {"pr", 1}}, p.WorkAuthorization)
	assert.Equal(t, []Count{{"Kuala Lumpur, Malaysia", 1}, {"Singapore", 1}}, p.Locations)
	assert.Equal(t, 30, *p.MedianNoticeDays)
	assert.Len(t, p.Salary, 2)
	assert.Equal(t, "MYR", p.Salary[0].Currency, "the currency most recorded first")
	assert.Equal(t, "12001", Amount(p.Salary[0].Expected.Median))
	assert.Nil(t, Build(nil, nil).MedianNoticeDays)
}

func TestMoney(t *testing.T) {
	assert.Equal(t, "RM 12,000", Money("MYR", 12000))
	assert.Equal(t, "SGD 950", Money("SGD", 949.6))
	assert.Equal(t, "RM 1,234,567", Money("MYR", 1234567))
}

func TestPoolTextNeverHasPay(t *testing.T) {
	p := Build([]Member{{Company: "Grab", Availability: "passive"}}, []Pay{{CurrentAmount: "9999", CurrentCurrency: "MYR", CurrentPeriod: "month"}})
	text := PoolText("finance", p)
	assert.Contains(t, text, `1 candidates have "finance" in their current title.`)
	assert.Contains(t, text, "Status: Passive 1")
	assert.NotContains(t, text, "9999")
	assert.Equal(t, `No candidates in our database have "x" in their current title.`, PoolText("x", Pool{}))
}

func TestDocLeavesOutPayWhenRestricted(t *testing.T) {
	p := Build([]Member{{Company: "Grab"}}, []Pay{{CurrentAmount: "9999", CurrentCurrency: "MYR", CurrentPeriod: "month"}})
	has := func(d simpledoc.Doc, s string) bool {
		for _, b := range d {
			if strings.Contains(b.Text, s) {
				return true
			}
		}
		return false
	}
	now := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	assert.True(t, has(Doc("Finance Manager", "finance", p, false, nil, now), "Current: RM 9,999 · RM 9,999 · RM 9,999 per month (1 candidates)"))
	assert.False(t, has(Doc("Finance Manager", "finance", p, true, nil, now), "9,999"))
	withAI := Doc("Finance Manager", "finance", p, true, &Insight{Text: "Talent supply\n- Tight.", AskedAt: now}, now)
	assert.True(t, has(withAI, "Market insights (AI)"))
	assert.True(t, has(withAI, "Tight."))
}
