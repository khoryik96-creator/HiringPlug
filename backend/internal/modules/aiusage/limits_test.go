package aiusage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWhatIsLeftIsTheLowestOfTheLimits(t *testing.T) {
	daily := int64(5000)
	s := Settings{Daily: &daily, PerFeature: map[string]int64{"cv_reading": 1000}}
	none := Spent{}
	assert.Equal(t, int64(9000), Within(9000, Settings{}, "cv_reading", Spent{Input: 4000}, none, 0), "no other limits: the month's")
	assert.Equal(t, int64(2000), Within(9000, s, "assistant", Spent{Input: 2500, Output: 500}, none, 0), "today's 5,000, 3,000 used")
	assert.Equal(t, int64(200), Within(9000, s, "cv_reading", none, Spent{Input: 600, Output: 200}, 0), "the feature's 1,000, 800 used")
	assert.Equal(t, int64(-800), Within(9000, s, "cv_reading", none, Spent{Input: 800}, 1000), "work in progress counts against each")
	assert.Equal(t, int64(100), Within(100, s, "assistant", none, none, 0), "the month's is lowest")
}

func TestTheDayStartsAtMidnightInTheAgencysTime(t *testing.T) {
	kl := time.FixedZone("MYT", 8*3600)
	now := time.Date(2026, 11, 26, 17, 30, 0, 0, time.UTC) // 01:30 on the 27th in Kuala Lumpur
	assert.Equal(t, time.Date(2026, 11, 27, 0, 0, 0, 0, kl), DayStart(now, kl))
}

func TestLimitsAreChecked(t *testing.T) {
	zero, big := int64(0), int64(MaxLimitTokens+1)
	for _, l := range []*Limits{{Daily: &zero}, {Daily: &big}, {PerFeature: map[string]int64{"coffee": 10}}, {PerFeature: map[string]int64{"cv_reading": 0}},
		{PerFeature: map[string]int64{FeatureProviderCheck: 1000}}} {
		in := SettingsInput{LimitKind: KindTokens, Currency: "USD", WarnPercent: 80, Limits: l}
		assert.Error(t, CheckSettings(in))
	}
	ok := int64(5000)
	assert.NoError(t, CheckSettings(SettingsInput{LimitKind: KindTokens, Currency: "USD", WarnPercent: 80,
		Limits: &Limits{Daily: &ok, PerFeature: map[string]int64{"cv_reading": 1000}}}))
}
