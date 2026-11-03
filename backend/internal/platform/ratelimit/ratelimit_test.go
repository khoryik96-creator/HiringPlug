package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimits(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(func() time.Time { return now }, Limit{Window: 10 * time.Minute, Max: 2}, Limit{Window: time.Hour, Max: 3})
	assert.True(t, l.Allow("a"))
	assert.True(t, l.Allow("a"))
	assert.False(t, l.Allow("a"), "two per ten minutes")
	assert.True(t, l.Allow("b"), "per key")
	now = now.Add(11 * time.Minute)
	assert.True(t, l.Allow("a"))
	now = now.Add(11 * time.Minute)
	assert.False(t, l.Allow("a"), "three per hour")
	now = now.Add(time.Hour)
	assert.True(t, l.Allow("a"))
}
