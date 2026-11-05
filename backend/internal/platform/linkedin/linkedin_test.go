package linkedin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"linkedin.com/in/siti":                            "https://www.linkedin.com/in/siti",
		"https://my.linkedin.com/in/siti-aisyah-123/?x=1": "https://www.linkedin.com/in/siti-aisyah-123",
		"lnkd.in/abc":                                     "https://lnkd.in/abc",
	} {
		got, ok := Normalize(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"linkedin.com", "example.com/in/siti", "not a url"} {
		_, ok := Normalize(bad)
		assert.False(t, ok, bad)
	}
}
