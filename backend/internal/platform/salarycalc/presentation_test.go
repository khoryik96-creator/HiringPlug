package salarycalc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatBreakdownForEmail(t *testing.T) {
	for _, tc := range []struct{ raw, currency, period, want string }{
		{"12.345678k", "MYR", "month", "RM 12,345.678 per month"},
		{"", "MYR", "month", ""},
		{"12k + 2 months fixed bonus", "MYR", "month", "RM 12,000 + 2 months fixed/contractual bonus per month"},
		{"12k + 13.5 bonus", "MYR", "month", "RM 12,000 + 13.5 bonus (component needs review) per month"},
		{"12,34 + 400 allowance", "MYR", "month", "12,34 (amount needs review) + RM 400 fixed allowance per month"},
		{"12", "MYR", "month", "RM 12 per month"},
		{"12k + 3200 alowance + 13.5 months total + perf bonus + epf13%", "MYR", "month", "RM 12,000 + RM 3,200 fixed allowance + 13.5th month fixed/contractual bonus + performance bonus + 13% EPF per month"},
		{"16k monthly + 400 allowance + AWS + 3 months performance + 16% epf", "MYR", "month", "RM 16,000 + RM 400 fixed allowance + 13th month fixed/contractual bonus + 3 months performance + 16% EPF per month"},
		{"12.5k + 450.50 fixed allowance + 14th bonus", "SGD", "month", "SGD 12,500 + SGD 450.50 fixed allowance + 14th month fixed/contractual bonus per month"},
		{"120k per year + 400 allowance", "MYR", "month", "RM 120,000 per year + RM 400 fixed allowance"},
		{"12k + 200 shift allowance + 2 months variable bonus", "MYR", "month", "RM 12,000 + RM 200 shift allowance + 2 months variable bonus per month"},
		{"12k + SGD 400 allowance", "MYR", "month", "RM 12,000 + SGD 400 fixed allowance per month"},
		{"12k + 13.5 + epf13", "MYR", "month", "RM 12,000 + 13.5 (component needs review) + EPF 13 (unit not specified) per month"},
		// "per month" belongs to an amount; wording without one stays as written.
		{"personal loan deducted", "MYR", "month", "personal loan deducted"},
		{"contract role, permanent later", "MYR", "month", "contract role, permanent later"},
		{"16% EPF", "MYR", "month", "16% EPF"},
		{"RM 12,000 + RM 3,200 fixed allowance + 13.5th month fixed/contractual bonus + performance bonus + 13% EPF per month", "MYR", "month", "RM 12,000 + RM 3,200 fixed allowance + 13.5th month fixed/contractual bonus + performance bonus + 13% EPF per month"},
	} {
		got := FormatBreakdown(tc.raw, tc.currency, tc.period)
		assert.Equal(t, tc.want, got, tc.raw)
	}
}
