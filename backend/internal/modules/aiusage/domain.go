package aiusage

// The rules, as pure functions on plain values (unit tested): the features,
// counts kept in range, months in the agency's time zone, the settings'
// checks, and the money maths. Money never passes through floating point:
// amounts are whole hundredths of the currency, prices whole ten-thousandths
// per million tokens, and every sum is done on big integers.

import (
	"math"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// What the AI is used for (the contract's AiFeature).
const (
	FeatureAssistant      = "assistant"
	FeatureAssistantEmail = "assistant_email"
	FeatureFitScore       = "fit_score"
	FeatureTalentSearch   = "talent_search"
	FeatureCVReading      = "cv_reading"
	FeatureCVStudio       = "cv_studio"
	FeatureJDBlind        = "jd_blind"
	FeatureJDReading      = "jd_reading"
	FeatureIndustryPick   = "industry_pick"
	FeatureInterviewPrep  = "interview_prep"
	FeatureProviderCheck  = "provider_check"
)

// Features lists every feature.
var Features = []string{
	FeatureAssistant, FeatureAssistantEmail, FeatureFitScore, FeatureTalentSearch, FeatureCVReading, FeatureCVStudio,
	FeatureJDBlind, FeatureJDReading, FeatureIndustryPick, FeatureInterviewPrep, FeatureProviderCheck,
}

// IsFeature says whether f is a known feature.
func IsFeature(f string) bool { return slices.Contains(Features, f) }

// Limit kinds.
const (
	KindTokens = "tokens"
	KindMoney  = "money"
)

// MaxTokens is the most one AI run may count, input or output: a result
// reporting more (or less than nothing) is out of range, not a real run.
const MaxTokens = 1 << 30

// ClampTokens keeps a reported count within 0..MaxTokens.
func ClampTokens(n int64) int64 { return min(max(n, 0), MaxTokens) }

// MaxRunKey is the longest source ID or run key kept.
const MaxRunKey = 80

// --- Months -------------------------------------------------------------

// MonthStart is the first moment of now's calendar month in loc (the
// agency's time zone): the month the allowance counts.
func MonthStart(now time.Time, loc *time.Location) time.Time {
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
}

// NextMonthStart is when now's month's allowance resets.
func NextMonthStart(now time.Time, loc *time.Location) time.Time {
	return MonthStart(now, loc).AddDate(0, 1, 0)
}

// MonthKey names t's month in loc, e.g. "2026-11".
func MonthKey(t time.Time, loc *time.Location) string { return t.In(loc).Format("2006-01") }

// MonthKeys names now's month and the n-1 before it, newest first.
func MonthKeys(now time.Time, loc *time.Location, n int) []string {
	start := MonthStart(now, loc)
	out := make([]string, n)
	for i := range n {
		out[i] = MonthKey(start.AddDate(0, -i, 0), loc)
	}
	return out
}

// --- Amounts and prices ------------------------------------------------

// Decimal places: amounts (the money limit) and prices (per million tokens).
const (
	AmountPlaces = 2
	PricePlaces  = 4
	costPlaces   = 4
)

var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

// ParseDecimal reads a non-negative decimal with at most places decimals and
// intDigits whole digits as a whole number of 10^-places, e.g. "0.27" with 4
// places is 2700.
func ParseDecimal(s string, places, intDigits int) (int64, bool) {
	if !decimalPattern.MatchString(s) {
		return 0, false
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(whole) > intDigits || len(frac) > places {
		return 0, false
	}
	n := new(big.Int)
	n.SetString(whole+frac+strings.Repeat("0", places-len(frac)), 10)
	if !n.IsInt64() {
		return 0, false
	}
	return n.Int64(), true
}

// FormatDecimal writes a whole number of 10^-places with every place, e.g.
// 850000 with 2 places is "8500.00".
func FormatDecimal(n int64, places int) string {
	return formatBig(big.NewInt(n), places)
}

func formatBig(n *big.Int, places int) string {
	s := n.String()
	if len(s) <= places {
		s = strings.Repeat("0", places-len(s)+1) + s
	}
	return s[:len(s)-places] + "." + s[len(s)-places:]
}

// --- The limit ----------------------------------------------------------

// Limit is a company's monthly limit, as it applies (the server's default
// already put in for a token limit the company has not set).
type Limit struct {
	Kind        string
	Tokens      int64 // the token limit (KindTokens)
	Money       int64 // the money limit in hundredths (KindMoney)
	Prices      Prices
	WarnPercent int
}

// Prices are what a million tokens cost, in ten-thousandths of the currency.
type Prices struct {
	Set           bool // both prices are set; costs are unknown otherwise
	Input, Output int64
}

// Spent is what was used: input and output tokens.
type Spent struct{ Input, Output int64 }

// Tokens is input and output together.
func (s Spent) Tokens() int64 { return s.Input + s.Output }

// Cost units: a token at a price per million in ten-thousandths costs
// price × 10^-10 of the currency. All money maths is done in these units.
var (
	unitsPerHundredth = big.NewInt(100_000_000)   // 10^-2 / 10^-10
	unitsPerCostPlace = big.NewInt(1_000_000)     // 10^-4 / 10^-10
	halfCostPlace     = big.NewInt(1_000_000 / 2) // for rounding half up
)

func mul(a, b int64) *big.Int { return new(big.Int).Mul(big.NewInt(a), big.NewInt(b)) }

// costUnits is what s cost at p.
func costUnits(p Prices, s Spent) *big.Int {
	return new(big.Int).Add(mul(s.Input, p.Input), mul(s.Output, p.Output))
}

// Cost is what s cost at p, to 4 decimal places (rounded half up); nil when
// the prices are not set.
func Cost(p Prices, s Spent) *string {
	if !p.Set {
		return nil
	}
	c := costUnits(p, s)
	c.Add(c, halfCostPlace).Quo(c, unitsPerCostPlace)
	out := formatBig(c, costPlaces)
	return &out
}

func (p Prices) higher() int64 { return max(p.Input, p.Output) }

func clampInt64(n *big.Int) int64 {
	switch {
	case n.IsInt64():
		return n.Int64()
	case n.Sign() > 0:
		return math.MaxInt64
	default:
		return math.MinInt64
	}
}

// Remaining is the month's allowance left, in tokens, after what was spent
// and the holds of work still in progress (pending tokens). For a money
// limit it is the money left (after pending tokens at the higher price) in
// tokens at the higher price, rounded down: a run never costs more than
// that. Zero or less: the limit is reached.
func Remaining(l Limit, spent Spent, pending int64) int64 {
	if l.Kind != KindMoney {
		left := new(big.Int).Sub(big.NewInt(l.Tokens), big.NewInt(spent.Tokens()))
		return clampInt64(left.Sub(left, big.NewInt(pending)))
	}
	top := l.Prices.higher()
	if top <= 0 {
		return math.MaxInt64 // nothing costs anything
	}
	left := new(big.Int).Mul(big.NewInt(l.Money), unitsPerHundredth)
	left.Sub(left, costUnits(l.Prices, spent))
	left.Sub(left, mul(pending, top))
	// Div rounds towards minus infinity for a positive divisor: down.
	return clampInt64(left.Div(left, big.NewInt(top)))
}

// used and limit in the same units (tokens, or money's cost units).
func (l Limit) usedAndLimit(spent Spent) (used, limit *big.Int) {
	if l.Kind == KindMoney {
		return costUnits(l.Prices, spent), new(big.Int).Mul(big.NewInt(l.Money), unitsPerHundredth)
	}
	return big.NewInt(spent.Tokens()), big.NewInt(l.Tokens)
}

// MaxUsedPercent caps UsedPercent (a tiny limit can be passed many times over).
const MaxUsedPercent = 1_000_000

// UsedPercent is the share of the limit spent, rounded down (it can pass
// 100, up to MaxUsedPercent). A limit of nothing counts as fully used.
func UsedPercent(l Limit, spent Spent) int64 {
	used, limit := l.usedAndLimit(spent)
	if limit.Sign() <= 0 {
		return 100
	}
	return min(clampInt64(used.Mul(used, big.NewInt(100)).Quo(used, limit)), MaxUsedPercent)
}

// Reached says whether the whole limit is spent (work in progress aside).
func Reached(l Limit, spent Spent) bool {
	used, limit := l.usedAndLimit(spent)
	return used.Cmp(limit) >= 0
}

// --- Settings -----------------------------------------------------------

// Defaults for a company that has not saved its settings.
const (
	DefaultCurrency    = "USD"
	DefaultWarnPercent = 80
	MaxLimitTokens     = 1_000_000_000_000
	amountDigits       = 12 // as the contract's DecimalAmount
	priceDigits        = 10 // as the contract's AiPrice
)

// SettingsInput is what an owner sends: amounts and prices as decimal text.
type SettingsInput struct {
	LimitKind             string
	LimitTokens           *int64 // nil: the server's default
	LimitMoney            *string
	Currency              string
	PriceInputPerMillion  *string
	PriceOutputPerMillion *string
	WarnPercent           int
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// CheckSettings lists what is wrong with in (none: nil error). A money limit
// needs the amount and both prices, at least one above zero.
func CheckSettings(in SettingsInput) error {
	var errs apperr.FieldErrors
	if in.LimitKind != KindTokens && in.LimitKind != KindMoney {
		errs.Add("limitKind", "must be tokens or money")
	}
	if in.LimitTokens != nil && (*in.LimitTokens < 0 || *in.LimitTokens > MaxLimitTokens) {
		errs.Add("limitTokens", "must be between 0 and 1,000,000,000,000")
	}
	if !currencyPattern.MatchString(in.Currency) {
		errs.Add("currency", "must be a 3-letter currency code, e.g. USD")
	}
	if in.WarnPercent < 1 || in.WarnPercent > 99 {
		errs.Add("warnPercent", "must be between 1 and 99")
	}
	money := in.LimitKind == KindMoney
	const needed = "is needed for a limit in money"
	amount := func(field string, v *string, places, digits int, format string) (int64, bool) {
		switch {
		case v == nil && money:
			errs.Add(field, needed)
		case v != nil:
			n, ok := ParseDecimal(*v, places, digits)
			if !ok {
				errs.Add(field, format)
			}
			return n, ok
		}
		return 0, false
	}
	amount("limitMoney", in.LimitMoney, AmountPlaces, amountDigits, "must be an amount with at most 2 decimal places, e.g. 50.00")
	const priceFormat = "must be a price with at most 4 decimal places, e.g. 0.27"
	priceIn, okIn := amount("priceInputPerMillion", in.PriceInputPerMillion, PricePlaces, priceDigits, priceFormat)
	priceOut, okOut := amount("priceOutputPerMillion", in.PriceOutputPerMillion, PricePlaces, priceDigits, priceFormat)
	if money && okIn && okOut && priceIn == 0 && priceOut == 0 {
		errs.Add("priceOutputPerMillion", "a limit in money needs a price above 0")
	}
	return errs.Err()
}

// Settings are a company's saved settings as values.
type Settings struct {
	Kind        string
	Tokens      *int64 // nil: the server's default
	Money       *int64 // hundredths
	Currency    string
	PriceInput  *int64 // ten-thousandths per million tokens
	PriceOutput *int64
	WarnPercent int
	Version     int
	UpdatedAt   *time.Time
}

// DefaultSettings are a company's settings before it saves any.
func DefaultSettings() Settings {
	return Settings{Kind: KindTokens, Currency: DefaultCurrency, WarnPercent: DefaultWarnPercent}
}

// Prices returns the settings' prices (Set only when both are).
func (s Settings) Prices() Prices {
	if s.PriceInput == nil || s.PriceOutput == nil {
		return Prices{}
	}
	return Prices{Set: true, Input: *s.PriceInput, Output: *s.PriceOutput}
}

// Limit is the limit that applies, with serverDefault (HP_AI_MONTHLY_TOKENS)
// for a token limit the company has not set. A money limit without its
// amount or prices (which the checks prevent) falls back to tokens.
func (s Settings) Limit(serverDefault int64) Limit {
	l := Limit{Kind: KindTokens, Tokens: serverDefault, Prices: s.Prices(), WarnPercent: s.WarnPercent}
	if s.Kind == KindMoney && s.Money != nil && l.Prices.Set {
		l.Kind, l.Tokens, l.Money = KindMoney, 0, *s.Money
		return l
	}
	if s.Tokens != nil {
		l.Tokens = *s.Tokens
	}
	return l
}

// --- The report ---------------------------------------------------------

// MonthRow is what one feature used in one month (from the ledger).
type MonthRow struct {
	Month, Feature string
	Uses           int64
	Spent          Spent
}

// FeatureUse is what one feature used.
type FeatureUse struct {
	Feature string
	Uses    int64
	Spent   Spent
}

// MonthUse is what a month used, in all and per feature (most tokens first).
type MonthUse struct {
	Month    string
	Uses     int64
	Spent    Spent
	Features []FeatureUse
}

// MonthsOf puts rows into the months named by keys (newest first), months
// without use included; rows of other months are left out.
func MonthsOf(keys []string, rows []MonthRow) []MonthUse {
	out := make([]MonthUse, len(keys))
	at := make(map[string]int, len(keys))
	for i, k := range keys {
		out[i] = MonthUse{Month: k, Features: []FeatureUse{}}
		at[k] = i
	}
	for _, r := range rows {
		i, ok := at[r.Month]
		if !ok {
			continue
		}
		m := &out[i]
		m.Uses += r.Uses
		m.Spent.Input += r.Spent.Input
		m.Spent.Output += r.Spent.Output
		m.Features = append(m.Features, FeatureUse{Feature: r.Feature, Uses: r.Uses, Spent: r.Spent})
	}
	for i := range out {
		slices.SortFunc(out[i].Features, func(a, b FeatureUse) int {
			if c := cmpDesc(a.Spent.Tokens(), b.Spent.Tokens()); c != 0 {
				return c
			}
			return strings.Compare(a.Feature, b.Feature)
		})
	}
	return out
}

func cmpDesc(a, b int64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}

// Names in the report for what the system did on its own, and for someone
// no longer among the company's users.
const (
	AutomaticName  = "Automatic"
	FormerUserName = "Former user"
)

// PersonUse is what one person (UserID nil: the system on its own) used.
type PersonUse struct {
	UserID *string
	Name   string
	Uses   int64
	Spent  Spent
}

// SortPeople puts the people who used most first (then by name).
func SortPeople(people []PersonUse) {
	slices.SortFunc(people, func(a, b PersonUse) int {
		if c := cmpDesc(a.Spent.Tokens(), b.Spent.Tokens()); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
}
