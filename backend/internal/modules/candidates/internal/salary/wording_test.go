package salary

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanWordingPreservesAmountsAndUnknownTerms(t *testing.T) {
	for _, tc := range [][2]string{
		{"16k + 400 alowance + perf bonus + epf13%", "16k + 400 allowance + performance bonus + 13% EPF"},
		{"EPF13,000", "EPF 13,000 (unit not specified)"},
		{"epf13 + AWS", "EPF 13 (unit not specified) + AWS"},
		{"RM 12,345.67 + 3 months performance bonus; paid by Acme", "RM 12,345.67 + 3 months performance bonus; paid by Acme"},
		{"no contractual bonus; shift allowance subject to attendance", "no contractual bonus; shift allowance subject to attendance"},
	} {
		got := CleanWording(tc[0])
		assert.Equal(t, tc[1], got)
		assert.Equal(t, got, CleanWording(got))
	}
}
