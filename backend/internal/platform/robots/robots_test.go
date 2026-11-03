package robots

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllows(t *testing.T) {
	robots := "User-agent: *\nDisallow: /careers/private\nAllow: /careers/private/open\n\nUser-agent: BadBot\nDisallow: /\n"
	assert.True(t, Allows(robots, "/careers"))
	assert.False(t, Allows(robots, "/careers/private/x"))
	assert.True(t, Allows(robots, "/careers/private/open/1"), "the longest rule wins")
	assert.False(t, Allows("User-agent: *\nDisallow: /\n", "/careers"))
	assert.False(t, Allows("User-agent: *\nAllow: /\nUser-agent: HiringPlugBot\nDisallow: /careers", "/careers"), "rules for us by name come first")
	assert.True(t, Allows("User-agent: *\nDisallow:\n", "/careers"))
	assert.True(t, Allows("", "/careers"))
}
