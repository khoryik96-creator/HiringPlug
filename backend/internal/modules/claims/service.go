package claims

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// The worker downloads the receipt soon after it is queued; a day is plenty
// even when the AI worker was down for a while.
const receiptLinkTTL = 24 * time.Hour

// Service holds the claims use cases.
type Service struct {
	pool  *pgxpool.Pool
	files *blob.Store
	deps  Deps
	zone  *time.Location
	now   func() time.Time
	// storage uploads submitted receipts (nil: not set up).
	storage *Storage
}

// today is the company's date, at midnight UTC (as dates are kept).
func (s *Service) today() time.Time {
	y, m, d := s.now().In(s.zone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// receiptKinds are the files a receipt may be: a PDF or a picture.
var receiptKinds = []string{filekind.PDF, filekind.JPEG, filekind.PNG, filekind.TIFF, filekind.BMP, filekind.WEBP}

// Upload keeps a receipt as a new draft claim of the person and queues
// reading it. The claim and the job are committed together.
func (s *Service) Upload(ctx context.Context, p authz.Principal, fileName string, data []byte) (api.Claim, error) {
	contentType, ok := filekind.Detect(fileName, data)
	if !ok || !slices.Contains(receiptKinds, contentType) {
		return api.Claim{}, apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "Upload a photo of the receipt (JPG, PNG, TIFF, BMP or WEBP) or a PDF.")
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	name := cleanFileName(fileName)
	var out api.Claim
	var storedKey string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if id, err := q.MyClaimWithReceipt(ctx, digest); err == nil {
			return apperr.New(api.ErrorCodeCLAIMDUPLICATE, "You already uploaded this receipt.").WithDetail("existingClaimId", id)
		} else if !database.IsNoRows(err) {
			return err
		}
		id, err := q.NewClaimID(ctx)
		if err != nil {
			return err
		}
		key := p.TenantID + "/" + id
		if err := s.files.Put(key, data); err != nil {
			return err
		}
		storedKey = key
		row, err := q.InsertClaim(ctx, store.InsertClaimParams{
			ID: id, FileName: name, ContentType: contentType, SizeBytes: int64(len(data)), Sha256: digest, StorageKey: key, Currency: DefaultCurrency,
		})
		if err != nil {
			return err
		}
		choices, err := s.deps.Options.Choices(ctx, tx, ReceiptTypeList)
		if err != nil {
			return err
		}
		types := make([]api.ClaimReceiptTypeOption, 0, len(choices))
		for _, c := range choices {
			types = append(types, api.ClaimReceiptTypeOption{Key: c.Key, Label: c.Label})
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindClaimReadReceipt, api.ClaimReadReceiptJob{
			ClaimId: id, DownloadUrl: s.files.SignedURL(key, receiptLinkTTL), ContentType: contentType,
			FileName: name, SizeBytes: int64(len(data)), ReceiptTypes: types,
		}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "claim.uploaded", EntityType: "claim", EntityID: id}); err != nil {
			return err
		}
		views, err := s.views(ctx, tx, p, []store.Claim{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	if err != nil && storedKey != "" {
		err = errors.Join(err, s.files.Delete(storedKey)) // rolled back: nothing refers to it
	}
	return out, err
}

// Filter narrows a list of claims.
type Filter struct {
	Status   *string
	UserID   *string // team lists only
	From, To *time.Time
	Limit    int
	Offset   int
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// Mine lists the person's own claims.
func (s *Service) Mine(ctx context.Context, p authz.Principal, f Filter) (api.ClaimPage, error) {
	out := api.ClaimPage{Items: []api.Claim{}, Totals: []api.ClaimAmount{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListMyClaims(ctx, store.ListMyClaimsParams{
			Status: f.Status, FromDate: pgDate(f.From), ToDate: pgDate(f.To), Lim: int32(f.Limit), Off: int32(f.Offset), //nolint:gosec // limited by the contract
		})
		if err != nil {
			return err
		}
		totals, err := q.MyClaimTotals(ctx, store.MyClaimTotalsParams{Status: f.Status, FromDate: pgDate(f.From), ToDate: pgDate(f.To)})
		if err != nil {
			return err
		}
		for _, t := range totals {
			out.Total += int(t.Claims)
			out.Totals = append(out.Totals, api.ClaimAmount{Currency: t.Currency, Amount: t.Amount})
		}
		out.Items, err = s.views(ctx, tx, p, rows)
		return err
	})
	return out, err
}

// Team lists everyone's claims but drafts (approvers).
func (s *Service) Team(ctx context.Context, p authz.Principal, f Filter) (api.ClaimPage, error) {
	out := api.ClaimPage{Items: []api.Claim{}, Totals: []api.ClaimAmount{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListTeamClaims(ctx, store.ListTeamClaimsParams{
			Status: f.Status, UserID: f.UserID, FromDate: pgDate(f.From), ToDate: pgDate(f.To), Lim: int32(f.Limit), Off: int32(f.Offset), //nolint:gosec // limited by the contract
		})
		if err != nil {
			return err
		}
		totals, err := q.TeamClaimTotals(ctx, store.TeamClaimTotalsParams{Status: f.Status, UserID: f.UserID, FromDate: pgDate(f.From), ToDate: pgDate(f.To)})
		if err != nil {
			return err
		}
		for _, t := range totals {
			out.Total += int(t.Claims)
			out.Totals = append(out.Totals, api.ClaimAmount{Currency: t.Currency, Amount: t.Amount})
		}
		out.Items, err = s.views(ctx, tx, p, rows)
		return err
	})
	return out, err
}

// Get returns one claim the person may see (their own; anyone's for approvers).
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (api.Claim, error) {
	var out api.Claim
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetClaim(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("claim")
		}
		if err != nil {
			return err
		}
		views, err := s.views(ctx, tx, p, []store.Claim{row})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

// lockOwn locks one of the person's own claims for a change.
func lockOwn(ctx context.Context, q *store.Queries, p authz.Principal, id string) (store.Claim, error) {
	row, err := q.LockClaim(ctx, id)
	if database.IsNoRows(err) {
		return row, apperr.NotFound("claim")
	}
	if err == nil && row.UserID != p.UserID {
		return row, apperr.New(api.ErrorCodeFORBIDDEN, "Only the person who claims it can change this claim.")
	}
	return row, err
}

func wrongStatus(status string) error {
	return apperr.New(api.ErrorCodeCLAIMWRONGSTATUS, "This claim is "+status+": only draft and rejected claims can be changed.")
}

// Update saves what the claimant changed on a draft or rejected claim.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, in Change) (api.Claim, error) {
	var out api.Claim
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := lockOwn(ctx, q, p, id)
		if err != nil {
			return err
		}
		if int(row.Version) != in.Version {
			return apperr.StaleVersion("claim")
		}
		if !Editable(row.Status) {
			return wrongStatus(row.Status)
		}
		v, err := valuesOf(row)
		if err != nil {
			return err
		}
		v, changed, err := s.apply(ctx, tx, p, v, in)
		if err != nil {
			return err
		}
		amount, err := numeric(v.Amount)
		if err != nil {
			return err
		}
		saved, err := q.SaveClaimFields(ctx, store.SaveClaimFieldsParams{
			ReceiptDate: pgDate(v.ReceiptDate), Amount: amount, Currency: v.Currency, Merchant: v.Merchant,
			ReceiptType: v.ReceiptType, ClaimType: v.ClaimType, Purpose: v.Purpose, LinkKind: v.LinkKind, LinkID: v.LinkID,
			FromReceipt: Unmark(row.FromReceipt, changed), ID: id, Version: row.Version,
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("claim")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "claim.updated", EntityType: "claim", EntityID: id,
			Changes: map[string]any{"fields": changed}}); err != nil {
			return err
		}
		views, err := s.views(ctx, tx, p, []store.Claim{saved})
		if err == nil {
			out = views[0]
		}
		return err
	})
	return out, err
}

// Delete removes the claimant's draft or rejected claim, and its receipt.
func (s *Service) Delete(ctx context.Context, p authz.Principal, id string) error {
	var key string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := lockOwn(ctx, q, p, id)
		if err != nil {
			return err
		}
		if !Editable(row.Status) {
			return apperr.New(api.ErrorCodeCLAIMWRONGSTATUS, "This claim is "+row.Status+": only draft and rejected claims can be deleted.")
		}
		gone, err := q.DeleteClaim(ctx, id)
		if err != nil {
			return err
		}
		key = gone.StorageKey
		if s.storage != nil {
			if err := s.storage.Deleted(ctx, tx, p.TenantID, gone); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "claim.deleted", EntityType: "claim", EntityID: id})
	})
	if err != nil {
		return err
	}
	if err := s.files.Delete(key); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Receipt opens a claim's receipt, one the person may see.
func (s *Service) Receipt(ctx context.Context, p authz.Principal, id string) (fileName string, f io.ReadCloser, size int64, err error) {
	var row store.Claim
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err = store.New(tx).GetClaim(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("claim")
		}
		return err
	})
	if err != nil {
		return "", nil, 0, err
	}
	file, err := s.files.Open(row.StorageKey)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, 0, apperr.NotFound("receipt")
	}
	if err != nil {
		return "", nil, 0, err
	}
	return row.FileName, file, row.SizeBytes, nil
}

// Summary returns the dashboard's figures: the person's own claims, or
// (team) everyone's but drafts.
func (s *Service) Summary(ctx context.Context, p authz.Principal, team bool, from, to *time.Time) (api.ClaimSummary, error) {
	out := api.ClaimSummary{Facts: []api.ClaimFact{}, People: []api.ClaimPerson{}, Records: []api.ClaimRecord{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ClaimFacts(ctx, store.ClaimFactsParams{Team: team, FromDate: pgDate(from), ToDate: pgDate(to)})
		if err != nil {
			return err
		}
		var users []string
		links := map[string][]string{}
		for _, r := range rows {
			fact := api.ClaimFact{Status: api.ClaimStatus(r.Status), Month: r.Month, Currency: r.Currency, Count: int(r.Claims),
				Amount: r.Amount, UserId: r.UserID, ReceiptType: nullableOf(r.ReceiptType), ClaimType: nullableOf(r.ClaimType),
				LinkId: nullableOf(r.LinkID), LinkKind: nullableOf[api.ClaimLinkKind](nil)}
			if r.LinkKind != nil {
				fact.LinkKind.Set(api.ClaimLinkKind(*r.LinkKind))
				if r.LinkID != nil && !slices.Contains(links[*r.LinkKind], *r.LinkID) {
					links[*r.LinkKind] = append(links[*r.LinkKind], *r.LinkID)
				}
			}
			if !slices.Contains(users, r.UserID) {
				users = append(users, r.UserID)
			}
			out.Facts = append(out.Facts, fact)
		}
		names, err := s.deps.Users.UserNames(ctx, tx, users)
		if err != nil {
			return err
		}
		for _, id := range users {
			out.People = append(out.People, api.ClaimPerson{Id: id, Name: names[id]})
		}
		found, err := s.records(ctx, tx, p, links)
		if err != nil {
			return err
		}
		for kind, ids := range links {
			for _, id := range ids {
				rec := api.ClaimRecord{Kind: api.ClaimLinkKind(kind), Id: id, Name: nullableOf[string](nil), Detail: nullableOf[string](nil)}
				if r, ok := found[kind][id]; ok {
					rec.Name.Set(r.Name)
					rec.Detail = nullableOf(r.Detail)
				}
				out.Records = append(out.Records, rec)
			}
		}
		slices.SortFunc(out.Records, func(a, b api.ClaimRecord) int {
			if a.Kind != b.Kind {
				return compare(string(a.Kind), string(b.Kind))
			}
			return compare(a.Id, b.Id)
		})
		return nil
	})
	return out, err
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
