package salary

import (
	"math/big"
	"strings"
)

// Money is an exact amount: every sum, division by 12 and percentage is done
// on fractions, never on binary floating point, so a rounded figure is the
// one a person would get on paper. The zero value is zero.
type Money struct{ r *big.Rat }

var (
	twelve   = big.NewRat(12, 1)
	hundred  = big.NewRat(100, 1)
	maxMoney = mustRat("999999999999.99")
)

func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("salary: bad constant " + s)
	}
	return r
}

// decimal reads a plain decimal such as "12.5" exactly.
func decimal(s string) (*big.Rat, bool) {
	if s == "" || strings.ContainsAny(s, "eE/") {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}

func moneyOf(r *big.Rat) Money { return Money{r: new(big.Rat).Set(r)} }

func (m Money) rat() *big.Rat {
	if m.r == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(m.r)
}

// String is the amount with two decimals ("18400.00"), as the API sends it.
func (m Money) String() string { return m.rat().FloatString(2) }

// Float64 is for comparisons in tests and display only, never for arithmetic.
func (m Money) Float64() float64 {
	f, _ := m.rat().Float64()
	return f
}

// rounded rounds half away from zero to cents or whole units.
func rounded(n *big.Rat, cents bool) *big.Rat {
	digits := 0
	if cents {
		digits = 2
	}
	return mustRat(n.FloatString(digits))
}

func validMoney(n *big.Rat) bool { return n.Sign() >= 0 && n.Cmp(maxMoney) <= 0 }

func add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }
func mul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
func quo(a, b *big.Rat) *big.Rat { return new(big.Rat).Quo(a, b) }

// plain writes an exact amount without needless zeros: 12345.678, 1100, 450.50.
func plain(n *big.Rat) string {
	s := n.FloatString(6)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}
