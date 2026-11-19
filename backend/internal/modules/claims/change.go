package claims

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Change is what a person changes on their claim: a nil field stays as it
// is; Clear names the fields emptied.
type Change struct {
	Version     int
	ReceiptDate *time.Time
	Amount      *string
	Currency    *string
	Merchant    *string
	ReceiptType *string
	ClaimType   *string
	Purpose     *string
	LinkKind    *string
	LinkID      *string
	Clear       []string
}

// readPermission is what a person needs to see (and tag) each kind of record.
var readPermission = map[string]api.Permission{
	LinkCompany: api.PermissionCompaniesRead, LinkContact: api.PermissionContactsRead, LinkCandidate: api.PermissionCandidatesRead,
}

var kindWord = map[string]string{LinkCompany: "company", LinkContact: "contact", LinkCandidate: "candidate"}

// apply checks a change and returns the new values with the fields that
// changed.
func (s *Service) apply(ctx context.Context, tx pgx.Tx, p authz.Principal, v Values, in Change) (Values, []string, error) {
	before := v
	var bad apperr.FieldErrors
	for _, f := range in.Clear {
		switch f {
		case FieldReceiptDate:
			v.ReceiptDate = nil
		case FieldAmount:
			v.Amount = nil
		case FieldMerchant:
			v.Merchant = nil
		case FieldReceiptType:
			v.ReceiptType = nil
		case FieldClaimType:
			v.ClaimType = nil
		case FieldPurpose:
			v.Purpose = nil
		case FieldLink:
			v.LinkKind, v.LinkID = nil, nil
		}
	}
	if in.ReceiptDate != nil {
		if ValidReceiptDate(*in.ReceiptDate, s.today()) {
			v.ReceiptDate = in.ReceiptDate
		} else {
			bad.Add(FieldReceiptDate, "must be a day up to today")
		}
	}
	if in.Amount != nil {
		if a, ok := CleanAmount(*in.Amount); ok {
			v.Amount = &a
		} else {
			bad.Add(FieldAmount, "must be an amount above 0, e.g. 23.50")
		}
	}
	if in.Currency != nil {
		if ValidCurrency(*in.Currency) {
			v.Currency = *in.Currency
		} else {
			bad.Add(FieldCurrency, "must be a currency code, e.g. MYR")
		}
	}
	if in.Merchant != nil {
		if t, ok := CleanText(in.Merchant, MaxMerchant); ok {
			v.Merchant = t
		} else {
			bad.Add(FieldMerchant, "is too long")
		}
	}
	if in.Purpose != nil {
		if t, ok := CleanText(in.Purpose, MaxPurpose); ok {
			v.Purpose = t
		} else {
			bad.Add(FieldPurpose, "is too long")
		}
	}
	for _, o := range []struct {
		field, list string
		in          *string
		to          **string
	}{{FieldReceiptType, ReceiptTypeList, in.ReceiptType, &v.ReceiptType}, {FieldClaimType, ClaimTypeList, in.ClaimType, &v.ClaimType}} {
		if o.in == nil {
			continue
		}
		ok, err := s.deps.Options.HasOption(ctx, tx, o.list, *o.in)
		if err != nil {
			return v, nil, err
		}
		if !ok {
			bad.Add(o.field, "must be one of the company's "+Labels[o.field]+"s (Settings)")
			continue
		}
		key := *o.in
		*o.to = &key
	}
	if in.LinkKind != nil && in.LinkID != nil {
		kind, id := *in.LinkKind, *in.LinkID
		switch {
		case !ValidLink(kind, id):
			bad.Add(FieldLink, "must be a company, contact or candidate")
		case !p.Can(readPermission[kind]):
			bad.Add(FieldLink, "you may not see "+kindWord[kind]+"s")
		default:
			found, err := s.deps.Records.Records(ctx, tx, kind, []string{id})
			if err != nil {
				return v, nil, err
			}
			if _, ok := found[id]; !ok {
				bad.Add(FieldLink, "no such "+kindWord[kind]+" (or you may not see it)")
			} else {
				v.LinkKind, v.LinkID = &kind, &id
			}
		}
	}
	if err := bad.Err(); err != nil {
		return v, nil, err
	}
	return v, changedFields(before, v), nil
}

// changedFields names the fields whose value differs.
func changedFields(a, b Values) []string {
	same := func(x, y *string) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) }
	sameDay := func(x, y *time.Time) bool { return (x == nil) == (y == nil) && (x == nil || x.Equal(*y)) }
	out := []string{}
	if !sameDay(a.ReceiptDate, b.ReceiptDate) {
		out = append(out, FieldReceiptDate)
	}
	if !same(a.Amount, b.Amount) {
		out = append(out, FieldAmount)
	}
	if a.Currency != b.Currency {
		out = append(out, FieldCurrency)
	}
	for _, f := range []struct {
		name string
		x, y *string
	}{{FieldMerchant, a.Merchant, b.Merchant}, {FieldReceiptType, a.ReceiptType, b.ReceiptType},
		{FieldClaimType, a.ClaimType, b.ClaimType}, {FieldPurpose, a.Purpose, b.Purpose}} {
		if !same(f.x, f.y) {
			out = append(out, f.name)
		}
	}
	if !same(a.LinkKind, b.LinkKind) || !same(a.LinkID, b.LinkID) {
		out = append(out, FieldLink)
	}
	return out
}

// records names the tagged records the person may see, by kind and id.
func (s *Service) records(ctx context.Context, tx pgx.Tx, p authz.Principal, links map[string][]string) (map[string]map[string]Record, error) {
	out := map[string]map[string]Record{}
	kinds := make([]string, 0, len(links))
	for kind := range links {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	for _, kind := range kinds {
		if perm, ok := readPermission[kind]; !ok || !p.Can(perm) || len(links[kind]) == 0 {
			continue
		}
		found, err := s.deps.Records.Records(ctx, tx, kind, links[kind])
		if err != nil {
			return nil, err
		}
		out[kind] = found
	}
	return out, nil
}
