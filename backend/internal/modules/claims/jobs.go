package claims

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// saveReceiptRead handles claim.receipt_read: what the AI worker read from a
// receipt goes into the claim's fields still empty (whatever the claimant
// typed meanwhile stands). Each value is checked again here: a date ahead,
// an amount out of range or a type the company does not have is dropped.
func (s *Service) saveReceiptRead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var in api.ClaimReceiptReadJob
	if err := job.Decode(&in); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.LockClaim(ctx, in.ClaimId)
	if database.IsNoRows(err) {
		return nil // deleted meanwhile
	}
	if err != nil {
		return err
	}
	read, err := s.checkedRead(ctx, tx, in)
	if err != nil {
		return err
	}
	v, err := valuesOf(row)
	if err != nil {
		return err
	}
	var filled []string
	if Editable(row.Status) { // once submitted, what is claimed stands
		v, filled = Prefill(v, read, row.Version == 1) // version 1: nobody has touched it yet
		if v.ClaimType == nil && v.ReceiptType != nil {
			if suggested := ClaimTypeFor(*v.ReceiptType); suggested != "" {
				ok, err := s.deps.Options.HasOption(ctx, tx, ClaimTypeList, suggested)
				if err != nil {
					return err
				}
				if ok {
					v.ClaimType = &suggested
					filled = append(filled, FieldClaimType)
				}
			}
		}
	}
	amount, err := numeric(v.Amount)
	if err != nil {
		return err
	}
	status, problem := OcrDone, (*string)(nil)
	if in.Status == api.ClaimReadStatusUnreadable {
		status = OcrFailed
		msg := "No text could be read from this receipt: fill in its details."
		if m, err := in.ErrorMessage.Get(); err == nil && strings.TrimSpace(m) != "" {
			msg = m
		}
		problem = shortened(msg, MaxNote)
	}
	if _, err := q.SaveReceiptRead(ctx, store.SaveReceiptReadParams{
		ID: row.ID, ReceiptDate: pgDate(v.ReceiptDate), Amount: amount, Currency: v.Currency, Merchant: v.Merchant,
		ReceiptType: v.ReceiptType, ClaimType: v.ClaimType, FromReceipt: Marked(row.FromReceipt, filled),
		OcrStatus: status, OcrError: problem, OcrExtractor: shortened(in.Extractor, 200),
	}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "claim.receipt_read", EntityType: "claim", EntityID: row.ID,
		Changes: map[string]any{"status": status, "filled": filled}})
}

// shortened is s trimmed and cut to at most n characters; nil when empty.
func shortened(s string, n int) *string {
	t := strings.Join(strings.Fields(s), " ")
	if r := []rune(t); len(r) > n {
		t = string(r[:n])
	}
	if t == "" {
		return nil
	}
	return &t
}

// checkedRead keeps what was read that is valid for this company.
func (s *Service) checkedRead(ctx context.Context, tx pgx.Tx, in api.ClaimReceiptReadJob) (Read, error) {
	var r Read
	if d, err := in.ReceiptDate.Get(); err == nil && ValidReceiptDate(d.Time, s.today()) {
		day := d.Time
		r.ReceiptDate = &day
	}
	if a, err := in.Amount.Get(); err == nil {
		if clean, ok := CleanAmount(a); ok {
			r.Amount = &clean
		}
	}
	if c, err := in.Currency.Get(); err == nil && ValidCurrency(c) {
		r.Currency = &c
	}
	if m, err := in.Merchant.Get(); err == nil {
		if t, ok := CleanText(&m, MaxMerchant); ok {
			r.Merchant = t
		}
	}
	if k, err := in.ReceiptType.Get(); err == nil && k != "" {
		has, err := s.deps.Options.HasOption(ctx, tx, ReceiptTypeList, k)
		if err != nil {
			return r, err
		}
		if has {
			r.ReceiptType = &k
		}
	}
	return r, nil
}
