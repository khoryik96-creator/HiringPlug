package claims

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// The upload jobs talk to the provider (seconds to minutes) with no
// transaction open; each step that reads or writes the database has its own.

// OutsideJobHandlers are the module's jobs that call the storage provider.
func (m *Module) OutsideJobHandlers() jobqueue.OutsideHandlers {
	s := m.service.storage
	if s == nil {
		return jobqueue.OutsideHandlers{}
	}
	return jobqueue.OutsideHandlers{api.JobKindClaimUpload: s.upload, api.JobKindClaimRemoveUpload: s.removeUpload}
}

// uploadWork is what an upload needs, read in one short transaction.
type uploadWork struct {
	claim   store.Claim
	storage store.ClaimStorage
	path    []string
	skip    bool   // nothing to do (stale job, claim withdrawn)
	fail    string // why it cannot be uploaded (no account, needs reconnecting)
}

func (s *Storage) upload(ctx context.Context, job jobqueue.Job) error {
	var in api.ClaimUploadJob
	if err := job.Decode(&in); err != nil {
		return err
	}
	seq := int32(in.Sequence) //nolint:gosec // a sequence number
	sys := authz.System(job.TenantID)
	var w uploadWork
	err := database.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
		var err error
		w, err = s.prepare(ctx, tx, in.ClaimId, seq)
		return err
	})
	if err != nil || w.skip {
		return err
	}
	if w.fail != "" {
		return s.failed(ctx, sys, in.ClaimId, seq, w.fail)
	}
	err = s.send(ctx, w)
	var p *providerError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &p) && p.reconnect():
		if markErr := s.mark(ctx, sys, w.storage, "reconnect", p.Error()); markErr != nil {
			return markErr
		}
		return s.failed(ctx, sys, in.ClaimId, seq, p.Error())
	case errors.As(err, &p) && p.transient() && job.Attempts < job.MaxAttempts:
		return err // tried again later
	}
	logx.From(ctx).Warn("claim receipt upload failed", "claim_id", in.ClaimId, "error", err)
	why := uploadFailed // the detail (paths, database errors) is for the log
	if errors.As(err, &p) {
		why = p.Error()
	}
	return s.failed(ctx, sys, in.ClaimId, seq, why)
}

// uploadFailed is what the claim shows when its upload failed for a reason
// that is not the provider's.
const uploadFailed = "The receipt could not be uploaded: try again later."

// DeadJobHandlers mark a claim's upload failed when its job is given up (an
// error on every attempt), so it shows and Settings can try it again.
func (m *Module) DeadJobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindClaimUpload: uploadDead}
}

func uploadDead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var in api.ClaimUploadJob
	if err := job.Decode(&in); err != nil {
		return nil //nolint:nilerr // an invalid job names no claim to mark
	}
	why := uploadFailed
	_, err := store.New(tx).FailClaimUpload(ctx, store.FailClaimUploadParams{
		ID: in.ClaimId, UploadSequence: int32(in.Sequence), UploadError: &why, //nolint:gosec // a sequence number
	})
	return err
}

func (s *Storage) prepare(ctx context.Context, tx pgx.Tx, id string, seq int32) (uploadWork, error) {
	q := store.New(tx)
	var w uploadWork
	row, err := q.GetClaim(ctx, id)
	if database.IsNoRows(err) {
		return uploadWork{skip: true}, nil // deleted since
	}
	if err != nil {
		return w, err
	}
	if row.UploadSequence != seq || row.UploadStatus != "pending" {
		return uploadWork{skip: true}, nil // submitted again since: a newer job uploads it
	}
	if row.Status == Draft || row.Status == Rejected {
		return uploadWork{skip: true}, q.ResetClaimUpload(ctx, store.ResetClaimUploadParams{ID: id, UploadSequence: seq}) // withdrawn
	}
	w.claim = row
	w.storage, err = q.GetClaimStorage(ctx)
	switch {
	case database.IsNoRows(err) || (err == nil && w.storage.Provider == nil):
		w.fail = "No claims storage is connected."
		return w, nil
	case err != nil:
		return w, err
	case w.storage.Status == "reconnect":
		w.fail = ProviderName(*w.storage.Provider) + " needs connecting again in Settings > Claims storage."
		return w, nil
	}
	v, err := valuesOf(row)
	if err != nil {
		return w, err
	}
	if v.ReceiptDate == nil || v.Amount == nil {
		w.fail = "The claim has no receipt date or amount."
		return w, nil
	}
	names, err := s.deps.Users.UserNames(ctx, tx, []string{row.UserID})
	if err != nil {
		return w, err
	}
	label := deref(v.ReceiptType)
	if s.deps.Options != nil && label != "" {
		labels, err := s.deps.Options.Labels(ctx, tx)
		if err != nil {
			return w, err
		}
		if l := labels[ReceiptTypeList][label]; l != "" {
			label = l
		}
	}
	w.path = UploadPath(Upload{
		Root: w.storage.RootFolder, Person: names[row.UserID], ReceiptType: label, Merchant: deref(v.Merchant),
		Currency: v.Currency, Amount: *v.Amount, ClaimID: row.ID, FileName: row.FileName, ContentType: row.ContentType,
		ReceiptDate: *v.ReceiptDate,
	})
	return w, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// send replaces the claim's earlier upload (same account) with this one.
func (s *Storage) send(ctx context.Context, w uploadWork) error {
	provider, account := *w.storage.Provider, *w.storage.Account
	token, err := s.access(ctx, w.storage)
	if err != nil {
		return err
	}
	f, err := s.files.Open(w.claim.StorageKey)
	if err != nil {
		return fmt.Errorf("the receipt could not be opened: %w", err)
	}
	data, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("the receipt could not be read: %w", err)
	}
	sys := authz.System(w.claim.TenantID)
	if old := w.claim.UploadRemoteID; old != nil && deref(w.claim.UploadProvider) == provider && deref(w.claim.UploadAccount) == account {
		if err := s.remove(ctx, provider, token, *old); err != nil {
			return err
		}
		if err := database.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
			return store.New(tx).ForgetClaimUpload(ctx, store.ForgetClaimUploadParams{ID: w.claim.ID, UploadRemoteID: old})
		}); err != nil {
			return err
		}
	}
	folders, name := w.path[:len(w.path)-1], w.path[len(w.path)-1]
	file, err := s.put(ctx, provider, token, folders, name, w.claim.ContentType, data)
	if err != nil {
		return err
	}
	where := strings.Join(w.path, "/")
	var link *string
	if u, err := url.Parse(file.URL); err == nil && u.Scheme == "https" && len(file.URL) <= 2000 {
		link = &file.URL
	}
	orphan := false
	err = database.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
		q := store.New(tx)
		saved, err := q.SaveClaimUpload(ctx, store.SaveClaimUploadParams{
			ID: w.claim.ID, UploadSequence: w.claim.UploadSequence, UploadPath: &where, UploadUrl: link,
			UploadProvider: &provider, UploadAccount: &account, UploadRemoteID: &file.ID,
		})
		if err != nil {
			return err
		}
		if saved == 0 { // deleted, withdrawn or sent again meanwhile
			held, err := q.ClaimHoldsUpload(ctx, store.ClaimHoldsUploadParams{UploadProvider: &provider, UploadRemoteID: &file.ID})
			orphan = !held
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "claim.uploaded", EntityType: "claim", EntityID: w.claim.ID,
			Changes: map[string]any{"provider": provider, "path": where}})
	})
	if err == nil && orphan {
		if err := s.remove(ctx, provider, token, file.ID); err != nil {
			logx.From(ctx).Warn("receipt of a claim changed during its upload left in the claims storage", "claim_id", w.claim.ID, "error", err)
		}
	}
	return err
}

func (s *Storage) failed(ctx context.Context, p authz.Principal, id string, seq int32, why string) error {
	why = *shortened(why, 500)
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		_, err := store.New(tx).FailClaimUpload(ctx, store.FailClaimUploadParams{ID: id, UploadSequence: seq, UploadError: &why})
		return err
	})
}

func (s *Storage) mark(ctx context.Context, p authz.Principal, row store.ClaimStorage, status, why string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		_, err := store.New(tx).MarkClaimStorage(ctx, store.MarkClaimStorageParams{Status: status, Error: shortened(why, 500), Provider: row.Provider, Account: row.Account})
		return err
	})
}

// access is an access token for the storage account, refreshed (and the
// new tokens kept) when it is about to expire.
func (s *Storage) access(ctx context.Context, row store.ClaimStorage) (string, error) {
	provider := *row.Provider
	var secret tokens
	if row.SealedTokens == nil || s.open(row.TenantID, "tokens", *row.SealedTokens, &secret) != nil {
		return "", &providerError{Provider: provider, Status: 401}
	}
	if secret.ExpiresAt.Sub(s.now()) > 2*time.Minute {
		return secret.AccessToken, nil
	}
	sys := authz.System(row.TenantID)
	var app App
	if err := database.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
		var err error
		app, err = s.appFor(ctx, tx, row.TenantID, provider)
		return err
	}); err != nil {
		return "", err
	}
	if app.ClientID == "" || app.ClientID != deref(row.AppClientID) {
		return "", &providerError{Provider: provider, Status: 401} // the application changed: its tokens are not valid with the new one
	}
	next, err := s.exchange(ctx, provider, app, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {secret.RefreshToken}})
	var p *providerError
	if errors.As(err, &p) && p.Status == 400 && p.Reason == "invalid_grant" {
		return "", &providerError{Provider: provider, Status: 401, Reason: p.Reason}
	}
	if err != nil {
		return "", err
	}
	if next.RefreshToken == "" {
		next.RefreshToken = secret.RefreshToken // Google and Dropbox keep the refresh token
	}
	sealed, err := s.seal(row.TenantID, "tokens", next)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, s.pool, sys, func(tx pgx.Tx) error {
		_, err := store.New(tx).SaveClaimStorageTokens(ctx, store.SaveClaimStorageTokensParams{SealedTokens: &sealed, Provider: row.Provider, Account: row.Account})
		return err
	})
	return next.AccessToken, err
}

// removeUpload deletes the receipt of a claim deleted or withdrawn, if its
// account is still the one connected and no claim records the file again.
func (s *Storage) removeUpload(ctx context.Context, job jobqueue.Job) error {
	var in api.ClaimRemoveUploadJob
	if err := job.Decode(&in); err != nil {
		return err
	}
	var row store.ClaimStorage
	held := false
	provider := string(in.Provider)
	err := database.InTx(ctx, s.pool, authz.System(job.TenantID), func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if held, err = q.ClaimHoldsUpload(ctx, store.ClaimHoldsUploadParams{UploadProvider: &provider, UploadRemoteID: &in.RemoteId}); err != nil || held {
			return err
		}
		row, err = q.GetClaimStorage(ctx)
		return err
	})
	if held || database.IsNoRows(err) || (err == nil && (deref(row.Provider) != provider || deref(row.Account) != in.Account || row.Status != "ok")) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := s.access(ctx, row)
	if err == nil {
		err = s.remove(ctx, string(in.Provider), token, in.RemoteId)
	}
	var p *providerError
	if err != nil && errors.As(err, &p) && p.transient() && job.Attempts < job.MaxAttempts {
		return err
	}
	if err != nil {
		logx.From(ctx).Warn("deleted claim's receipt left in the claims storage", "error", err)
	}
	return nil
}
