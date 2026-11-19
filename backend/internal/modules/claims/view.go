package claims

import (
	"context"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// nullableOf is v as a JSON value that may be null.
func nullableOf[T any](v *T) nullable.Nullable[T] {
	if v == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*v)
}

// valuesOf reads a claim's fields.
func valuesOf(row store.Claim) (Values, error) {
	amount, err := database.DecimalString(row.Amount)
	if err != nil {
		return Values{}, err
	}
	v := Values{Amount: amount, Currency: row.Currency, Merchant: row.Merchant, ReceiptType: row.ReceiptType,
		ClaimType: row.ClaimType, Purpose: row.Purpose, LinkKind: row.LinkKind, LinkID: row.LinkID}
	if row.ReceiptDate.Valid {
		d := row.ReceiptDate.Time
		v.ReceiptDate = &d
	}
	return v, nil
}

// numeric is an amount ("23.50") as the database keeps it.
func numeric(s *string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if s == nil {
		return n, nil
	}
	return n, n.Scan(*s)
}

// cleanFileName keeps a file's base name, without control characters, at
// most 255 characters.
func cleanFileName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	if name == "" {
		return "receipt"
	}
	return name
}

func fields(names []string) []api.ClaimField {
	out := make([]api.ClaimField, 0, len(names))
	for _, n := range names {
		out = append(out, api.ClaimField(n))
	}
	return out
}

// views are claims as the API shows them to p: with the names of the people
// and of the records tagged (those p may see).
func (s *Service) views(ctx context.Context, tx pgx.Tx, p authz.Principal, rows []store.Claim) ([]api.Claim, error) {
	var users []string
	links := map[string][]string{}
	for _, r := range rows {
		for _, u := range []*string{&r.UserID, r.ReviewedBy} {
			if u != nil && !slices.Contains(users, *u) {
				users = append(users, *u)
			}
		}
		if r.LinkKind != nil && r.LinkID != nil && !slices.Contains(links[*r.LinkKind], *r.LinkID) {
			links[*r.LinkKind] = append(links[*r.LinkKind], *r.LinkID)
		}
	}
	names, err := s.deps.Users.UserNames(ctx, tx, users)
	if err != nil {
		return nil, err
	}
	found, err := s.records(ctx, tx, p, links)
	if err != nil {
		return nil, err
	}
	out := make([]api.Claim, 0, len(rows))
	for _, r := range rows {
		v, err := valuesOf(r)
		if err != nil {
			return nil, err
		}
		c := api.Claim{
			Id: r.ID, UserId: r.UserID, UserName: names[r.UserID], Status: api.ClaimStatus(r.Status),
			OcrStatus: api.ClaimOcrStatus(r.OcrStatus), OcrError: nullableOf(r.OcrError), FileName: r.FileName,
			ContentType: r.ContentType, SizeBytes: r.SizeBytes, Amount: nullableOf(v.Amount), Currency: r.Currency,
			Merchant: nullableOf(r.Merchant), ReceiptType: nullableOf(r.ReceiptType), ClaimType: nullableOf(r.ClaimType),
			Purpose: nullableOf(r.Purpose), FromReceipt: fields(r.FromReceipt), Missing: fields(Missing(v)),
			ClaimedAt: nullableOf(r.ClaimedAt), ReviewedAt: nullableOf(r.ReviewedAt), ReviewNote: nullableOf(r.ReviewNote),
			PaidAt: nullableOf(r.PaidAt), ReviewedByName: nullable.NewNullNullable[string](), Link: nullable.NewNullNullable[api.ClaimLink](),
			Upload: api.ClaimUpload{Status: api.ClaimUploadStatus(r.UploadStatus), Path: nullableOf(r.UploadPath), Url: nullableOf(r.UploadUrl),
				Error: nullableOf(r.UploadError), UploadedAt: nullableOf(r.UploadedAt)},
			Version: int(r.Version), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ReceiptDate: nullable.NewNullNullable[openapi_types.Date](),
		}
		if v.ReceiptDate != nil {
			c.ReceiptDate.Set(openapi_types.Date{Time: *v.ReceiptDate})
		}
		if r.ReviewedBy != nil {
			c.ReviewedByName.Set(names[*r.ReviewedBy])
		}
		if r.LinkKind != nil && r.LinkID != nil {
			link := api.ClaimLink{Kind: api.ClaimLinkKind(*r.LinkKind), Id: *r.LinkID,
				Name: nullable.NewNullNullable[string](), Detail: nullable.NewNullNullable[string]()}
			if rec, ok := found[*r.LinkKind][*r.LinkID]; ok {
				link.Name.Set(rec.Name)
				link.Detail = nullableOf(rec.Detail)
			}
			c.Link.Set(link)
		}
		out = append(out, c)
	}
	return out, nil
}
