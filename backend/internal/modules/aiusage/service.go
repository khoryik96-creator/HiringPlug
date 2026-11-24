package aiusage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Service holds the module's use cases.
type Service struct {
	pool   *pgxpool.Pool
	deps   Deps
	config Config
	now    func() time.Time
}

func (s *Service) loc() *time.Location { return s.config.TimeZone }

// record inserts u as the system for tenantID; recorded is false when the
// run was already recorded or used nothing.
func (s *Service) record(ctx context.Context, tx pgx.Tx, tenantID string, u Use) (recorded bool, err error) {
	if !IsFeature(u.Feature) {
		return false, fmt.Errorf("aiusage: unknown feature %q", u.Feature)
	}
	if u.SourceID == "" || len(u.SourceID) > MaxRunKey || len(u.RunKey) > MaxRunKey {
		return false, fmt.Errorf("aiusage: source %q / run key %q out of range", u.SourceID, u.RunKey)
	}
	in, out := ClampTokens(u.Input), ClampTokens(u.Output)
	if in+out == 0 {
		return false, nil
	}
	user := u.UserID
	if user != nil && (*user == "" || *user == authz.SystemUserID) {
		user = nil
	}
	err = database.AsSystem(ctx, tx, tenantID, func() error {
		n, err := store.New(tx).RecordUse(ctx, store.RecordUseParams{
			Feature: u.Feature, UserID: user, SourceID: u.SourceID, RunKey: u.RunKey, InputTokens: in, OutputTokens: out,
		})
		recorded = n > 0
		return err
	})
	return recorded, err
}

// month reads the company's settings and this month's use, as the system.
func (s *Service) month(ctx context.Context, tx pgx.Tx, tenantID string) (Settings, Spent, error) {
	var set Settings
	var spent Spent
	err := database.AsSystem(ctx, tx, tenantID, func() error {
		q := store.New(tx)
		var err error
		if set, err = readSettings(ctx, q); err != nil {
			return err
		}
		use, err := q.UseSince(ctx, MonthStart(s.now(), s.loc()))
		spent = Spent{Input: use.InputTokens, Output: use.OutputTokens}
		return err
	})
	return set, spent, err
}

func (s *Service) remaining(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error) {
	// Pending first: it takes the company's allowance lock, so a result
	// landing between the two reads can only overestimate the use.
	pending, err := jobqueue.PendingAI(ctx, tx, tenantID)
	if err != nil {
		return 0, err
	}
	set, spent, err := s.month(ctx, tx, tenantID)
	if err != nil {
		return 0, err
	}
	return Remaining(set.Limit(s.config.DefaultTokens), spent, pending), nil
}

func (s *Service) status(ctx context.Context, tx pgx.Tx, tenantID string) (Status, error) {
	set, spent, err := s.month(ctx, tx, tenantID)
	if err != nil {
		return Status{}, err
	}
	return statusOf(set.Limit(s.config.DefaultTokens), spent, s.now(), s.loc()), nil
}

func statusOf(l Limit, spent Spent, now time.Time, loc *time.Location) Status {
	st := Status{
		Kind: l.Kind, UsedTokens: spent.Tokens(), UsedPercent: UsedPercent(l, spent), WarnPercent: l.WarnPercent,
		Reached: Reached(l, spent), MonthStart: MonthStart(now, loc), ResetsAt: NextMonthStart(now, loc),
	}
	if l.Kind == KindTokens {
		st.LimitTokens = l.Tokens
	}
	st.Warn = !st.Reached && st.UsedPercent >= int64(l.WarnPercent)
	return st
}

// --- Settings -----------------------------------------------------------

// numericValue reads a numeric column as a whole number of 10^-places.
func numericValue(n pgtype.Numeric, places int) (*int64, error) {
	text, err := database.DecimalString(n)
	if err != nil || text == nil {
		return nil, err
	}
	v, ok := ParseDecimal(*text, places, 20)
	if !ok {
		return nil, fmt.Errorf("aiusage: unexpected amount %q", *text)
	}
	return &v, nil
}

func readSettings(ctx context.Context, q *store.Queries) (Settings, error) {
	row, err := q.GetSetting(ctx)
	if database.IsNoRows(err) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	at := row.UpdatedAt
	out := Settings{Kind: row.LimitKind, Tokens: row.LimitTokens, Currency: row.Currency, WarnPercent: int(row.WarnPercent),
		Version: int(row.Version), UpdatedAt: &at}
	if out.Money, err = numericValue(row.LimitMoney, AmountPlaces); err != nil {
		return Settings{}, err
	}
	if out.PriceInput, err = numericValue(row.PriceInputPerMillion, PricePlaces); err != nil {
		return Settings{}, err
	}
	out.PriceOutput, err = numericValue(row.PriceOutputPerMillion, PricePlaces)
	return out, err
}

// SettingsView is the settings as shown: the saved ones and the server's default.
type SettingsView struct {
	Settings
	ServerDefaultTokens int64
}

// Settings returns the company's limit and prices.
func (s *Service) Settings(ctx context.Context, p authz.Principal) (SettingsView, error) {
	var out SettingsView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out.Settings, err = readSettings(ctx, store.New(tx))
		return err
	})
	out.ServerDefaultTokens = s.config.DefaultTokens
	return out, err
}

func scaled(v *string, places int) *int64 {
	if v == nil {
		return nil
	}
	n, _ := ParseDecimal(*v, places, 20) // checked by CheckSettings
	return &n
}

// normalized is the input as saved: amounts rewritten with every place, so
// "50" and "50.00" are the same.
func normalized(in SettingsInput) Settings {
	return Settings{Kind: in.LimitKind, Tokens: in.LimitTokens, Money: scaled(in.LimitMoney, AmountPlaces), Currency: in.Currency,
		PriceInput: scaled(in.PriceInputPerMillion, PricePlaces), PriceOutput: scaled(in.PriceOutputPerMillion, PricePlaces), WarnPercent: in.WarnPercent}
}

func text(v *int64, places int) *string {
	if v == nil {
		return nil
	}
	s := FormatDecimal(*v, places)
	return &s
}

// auditFields are the settings as the audit entry records them (company
// settings, no personal data or salary).
func auditFields(s Settings) map[string]any {
	return map[string]any{
		"limitKind": s.Kind, "limitTokens": s.Tokens, "limitMoney": text(s.Money, AmountPlaces), "currency": s.Currency,
		"priceInputPerMillion": text(s.PriceInput, PricePlaces), "priceOutputPerMillion": text(s.PriceOutput, PricePlaces),
		"warnPercent": s.WarnPercent,
	}
}

// SaveSettings sets the limit and prices; version is the one loaded (0
// before the first save), and an older one is refused.
func (s *Service) SaveSettings(ctx context.Context, p authz.Principal, in SettingsInput, version int) (SettingsView, error) {
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if err := CheckSettings(in); err != nil {
		return SettingsView{}, err
	}
	next := normalized(in)
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		before, err := readSettings(ctx, q)
		if err != nil {
			return err
		}
		money, priceIn, priceOut := text(next.Money, AmountPlaces), text(next.PriceInput, PricePlaces), text(next.PriceOutput, PricePlaces)
		if version == 0 {
			_, err = q.InsertSetting(ctx, store.InsertSettingParams{
				LimitKind: next.Kind, LimitTokens: next.Tokens, LimitMoney: money, Currency: next.Currency,
				PriceInputPerMillion: priceIn, PriceOutputPerMillion: priceOut, WarnPercent: int16(next.WarnPercent), //nolint:gosec // checked: 1..99
			})
		} else {
			_, err = q.UpdateSetting(ctx, store.UpdateSettingParams{
				LimitKind: next.Kind, LimitTokens: next.Tokens, LimitMoney: money, Currency: next.Currency,
				PriceInputPerMillion: priceIn, PriceOutputPerMillion: priceOut, WarnPercent: int16(next.WarnPercent), //nolint:gosec // checked: 1..99
				Version: int32(version), //nolint:gosec // a version number
			})
		}
		if database.IsNoRows(err) {
			return apperr.StaleVersion("AI usage settings")
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "ai_usage.settings_saved", EntityType: "tenant", EntityID: p.TenantID,
			Changes: audit.Diff(auditFields(before), auditFields(next)),
		})
	})
	if err != nil {
		return SettingsView{}, err
	}
	return s.Settings(ctx, p)
}

// UsageStatus is the company's use of this month's limit, for everyone.
func (s *Service) UsageStatus(ctx context.Context, p authz.Principal) (Status, error) {
	var out Status
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.status(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}
