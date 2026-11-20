package claims

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Claims storage: one company account (Google Drive, OneDrive or Dropbox)
// that owners and admins connect in Settings; each submitted claim's
// receipt is uploaded to it by the claim.upload job.

// App is an OAuth application registered with a provider.
type App struct{ ClientID, ClientSecret string }

// StorageApps gives the agency's Microsoft and Google applications (the
// ones its mailboxes connect through), in the caller's transaction.
type StorageApps interface {
	App(ctx context.Context, tx pgx.Tx, tenant, provider string) (App, error) // provider: "microsoft" or "google"
}

// StorageConfig is the deployment's part.
type StorageConfig struct {
	KeysSecret []byte // HP_MAIL_KEYS_SECRET: seals the account's tokens
	PublicURL  string // the provider sends people back to <PublicURL>/v1/claims/storage/callback
	Dropbox    App    // HP_DROPBOX_APP_KEY, HP_DROPBOX_APP_SECRET
	HTTP       *http.Client
	Endpoints  Endpoints // RealEndpoints when empty
}

// Storage uploads receipts to the company's claims storage.
type Storage struct {
	pool   *pgxpool.Pool
	files  *blob.Store
	deps   Deps
	apps   StorageApps
	config StorageConfig
	now    func() time.Time
}

// UseStorage turns on uploading receipts to the company's claims storage.
func (m *Module) UseStorage(apps StorageApps, c StorageConfig) {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 2 * time.Minute}
	}
	client := *c.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("storage provider redirect refused") }
	c.HTTP = &client
	if c.Endpoints.Graph == "" {
		c.Endpoints = RealEndpoints
	}
	s := m.service
	s.storage = &Storage{pool: s.pool, files: s.files, deps: s.deps, apps: apps, config: c, now: time.Now}
	m.HTTP.storage = s.storage
}

func (s *Storage) callbackURL() string {
	return strings.TrimRight(s.config.PublicURL, "/") + "/v1/claims/storage/callback"
}

// appFor is the application a provider is connected through.
func (s *Storage) appFor(ctx context.Context, tx pgx.Tx, tenant, provider string) (App, error) {
	switch provider {
	case Dropbox:
		return s.config.Dropbox, nil
	case GoogleDrive:
		return s.apps.App(ctx, tx, tenant, "google")
	}
	return s.apps.App(ctx, tx, tenant, "microsoft")
}

func storageProblem(message string) error {
	return apperr.Validation(apperr.FieldErrors{{Field: "storage", Message: message}})
}

// problem says why a provider cannot be connected (empty: it can).
func (s *Storage) problem(ctx context.Context, tx pgx.Tx, tenant, provider string) (string, error) {
	if len(s.config.KeysSecret) < 32 {
		return "The server's secure connection storage (HP_MAIL_KEYS_SECRET) is not configured.", nil
	}
	app, err := s.appFor(ctx, tx, tenant, provider)
	if err != nil {
		return "", err
	}
	if app.ClientID != "" && app.ClientSecret != "" {
		return "", nil
	}
	switch provider {
	case Dropbox:
		return "Set HP_DROPBOX_APP_KEY and HP_DROPBOX_APP_SECRET on the server (a Dropbox app with this redirect URL).", nil
	case GoogleDrive:
		return "Add the Google application in Settings > Email connection (with the Google Drive API on and this redirect URL).", nil
	}
	return "Add the Microsoft application in Settings > Email connection (with this redirect URL).", nil
}

// View is the storage as Settings shows it.
func (s *Storage) View(ctx context.Context, p authz.Principal) (api.ClaimStorage, error) {
	var out api.ClaimStorage
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.viewIn(ctx, tx, p)
		return err
	})
	return out, err
}

func (s *Storage) viewIn(ctx context.Context, tx pgx.Tx, p authz.Principal) (api.ClaimStorage, error) {
	q := store.New(tx)
	row, err := q.GetClaimStorage(ctx)
	if database.IsNoRows(err) {
		row, err = store.ClaimStorage{RootFolder: "Claims", Status: "disconnected"}, nil
	}
	if err != nil {
		return api.ClaimStorage{}, err
	}
	out := api.ClaimStorage{
		Provider: nullableOf((*api.ClaimStorageProvider)(row.Provider)), Account: nullableOf(row.Account), RootFolder: row.RootFolder,
		Status: api.ClaimStorageStatus(row.Status), Error: nullableOf(row.Error), CallbackUrl: s.callbackURL(),
		ConnectedAt: nullableOf(row.ConnectedAt), Version: int(row.Version), Options: []api.ClaimStorageOption{},
	}
	for _, provider := range []string{GoogleDrive, OneDrive, Dropbox} {
		why, err := s.problem(ctx, tx, p.TenantID, provider)
		if err != nil {
			return out, err
		}
		opt := api.ClaimStorageOption{Provider: api.ClaimStorageProvider(provider), Ready: why == "", Problem: nullableOf[string](nil)}
		if why != "" {
			opt.Problem = nullableOf(&why)
		}
		out.Options = append(out.Options, opt)
	}
	err = asSystem(ctx, tx, func(q *store.Queries) error { // everyone's claims, whoever manages the storage
		n, err := q.ClaimUploadCounts(ctx)
		out.Uploads = api.ClaimUploadCounts{Pending: int(n.Pending), Failed: int(n.Failed), Done: int(n.Done)}
		return err
	})
	return out, err
}

// SetFolder changes the top folder receipts go into (uploads from now on).
func (s *Storage) SetFolder(ctx context.Context, p authz.Principal, root string, version int) (api.ClaimStorage, error) {
	if !ValidRoot(root) {
		return api.ClaimStorage{}, apperr.Validation(apperr.FieldErrors{{Field: "rootFolder", Message: "name a folder, e.g. Claims or Finance/Claims (at most five deep)"}})
	}
	clean := strings.Join(RootFolders(root), "/")
	var out api.ClaimStorage
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		_, err := store.New(tx).SaveClaimStorageFolder(ctx, store.SaveClaimStorageFolderParams{RootFolder: clean, Version: int32(version)}) //nolint:gosec // a version number
		if database.IsNoRows(err) {
			return apperr.StaleVersion("claims storage")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "claim_storage.folder_changed", EntityType: "claim_storage", EntityID: p.TenantID, Changes: map[string]any{"rootFolder": clean}}); err != nil {
			return err
		}
		out, err = s.viewIn(ctx, tx, p)
		return err
	})
	return out, err
}

// Authorize starts connecting an account: the provider's sign-in page.
func (s *Storage) Authorize(ctx context.Context, p authz.Principal, provider string) (string, error) {
	var app App
	expires := s.now().Add(10 * time.Minute)
	state := s.signState(stateClaim{Tenant: p.TenantID, User: p.UserID, Provider: provider, Nonce: randomToken(), Expires: expires.Unix()})
	verifier := randomToken()
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		why, err := s.problem(ctx, tx, p.TenantID, provider)
		if err != nil {
			return err
		}
		if why != "" {
			return storageProblem(why)
		}
		if app, err = s.appFor(ctx, tx, p.TenantID, provider); err != nil {
			return err
		}
		sealed, err := s.seal(p.TenantID, "verifier:"+p.UserID, verifier)
		if err != nil {
			return err
		}
		return store.New(tx).SaveClaimStorageState(ctx, store.SaveClaimStorageStateParams{
			Provider: provider, StateHash: digest(state), AppClientID: app.ClientID, SealedVerifier: sealed, ExpiresAt: expires,
		})
	})
	if err != nil {
		return "", err
	}
	spec := s.spec(provider)
	target, err := url.Parse(spec.authorize)
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	q := target.Query()
	for k, v := range spec.extra {
		q[k] = v
	}
	q.Set("client_id", app.ClientID)
	q.Set("redirect_uri", s.callbackURL())
	q.Set("scope", spec.scope)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	target.RawQuery = q.Encode()
	return target.String(), nil
}

// Park keeps the code the provider sent back (the callback is public: the
// browser comes back without HiringPlug's sign-in), for the person who
// started the sign-in to finish with the ticket returned.
func (s *Storage) Park(ctx context.Context, state, code, denied string) (string, error) {
	claim, err := s.readState(state)
	if err != nil {
		return "", err
	}
	p := authz.NewPrincipalWithAccess(claim.Tenant, claim.User, "", "", api.RoleAdmin, []api.Permission{api.PermissionClaimsStorageManage}, authz.Scope{Users: []string{}})
	var sealedCode, ticketHash *string
	ticket := ""
	if code != "" && denied == "" {
		sealed, err := s.seal(claim.Tenant, "code:"+claim.User, code)
		if err != nil {
			return "", err
		}
		sealedCode, ticket = &sealed, randomToken()
		hash := digest(ticket)
		ticketHash = &hash
	}
	var changed int64
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		changed, err = store.New(tx).ParkClaimStorageCode(ctx, store.ParkClaimStorageCodeParams{StateHash: digest(state), SealedCode: sealedCode, TicketHash: ticketHash})
		return err
	})
	if err != nil || changed != 1 || ticket == "" {
		return "", errors.New("claims storage sign-in was not completed or expired")
	}
	return ticket, nil
}

// Complete connects the account the provider approved, and uploads again
// the receipts that could not be uploaded.
func (s *Storage) Complete(ctx context.Context, p authz.Principal, ticket string) (api.ClaimStorage, error) {
	problem := storageProblem("The sign-in was not completed or expired. Connect again.")
	var saved store.ClaimStorageState
	var app App
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		if saved, err = store.New(tx).ParkedClaimStorageCode(ctx, ptrTo(digest(ticket))); err != nil {
			return err
		}
		app, err = s.appFor(ctx, tx, p.TenantID, saved.Provider)
		return err
	})
	if database.IsNoRows(err) || (err == nil && saved.SealedCode == nil) {
		return api.ClaimStorage{}, problem
	}
	if err != nil {
		return api.ClaimStorage{}, err
	}
	var code, verifier string
	if s.open(p.TenantID, "code:"+p.UserID, *saved.SealedCode, &code) != nil || s.open(p.TenantID, "verifier:"+p.UserID, saved.SealedVerifier, &verifier) != nil {
		return api.ClaimStorage{}, problem
	}
	if app.ClientID != saved.AppClientID {
		return api.ClaimStorage{}, storageProblem("The " + ProviderName(saved.Provider) + " application changed during sign-in. Connect again.")
	}
	secret, err := s.exchange(ctx, saved.Provider, app, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.callbackURL()}, "code_verifier": {verifier}})
	if err != nil {
		return api.ClaimStorage{}, storageProblem(err.Error())
	}
	if secret.RefreshToken == "" {
		return api.ClaimStorage{}, storageProblem(ProviderName(saved.Provider) + " did not allow lasting access (offline access). Connect again and approve it.")
	}
	account, err := s.identity(ctx, saved.Provider, secret.AccessToken)
	if err != nil {
		return api.ClaimStorage{}, storageProblem(err.Error())
	}
	sealed, err := s.seal(p.TenantID, "tokens", secret)
	if err != nil {
		return api.ClaimStorage{}, err
	}
	var out api.ClaimStorage
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		taken, err := q.TakeClaimStorageCode(ctx, saved.StateHash)
		if err != nil {
			return err
		}
		if taken != 1 {
			return problem
		}
		if _, err := q.ConnectClaimStorage(ctx, store.ConnectClaimStorageParams{Provider: &saved.Provider, Account: &account, SealedTokens: &sealed, AppClientID: &app.ClientID}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "claim_storage.connected", EntityType: "claim_storage", EntityID: p.TenantID,
			Changes: map[string]any{"provider": saved.Provider, "account": account}}); err != nil {
			return err
		}
		if _, err := s.uploadAgain(ctx, tx, p.TenantID); err != nil {
			return err
		}
		out, err = s.viewIn(ctx, tx, p)
		return err
	})
	return out, err
}

// Disconnect forgets the account: nothing more is uploaded (what was stays).
func (s *Storage) Disconnect(ctx context.Context, p authz.Principal) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).DisconnectClaimStorage(ctx); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "claim_storage.disconnected", EntityType: "claim_storage", EntityID: p.TenantID})
	})
}

// Retry uploads again the receipts whose upload failed (or never finished).
func (s *Storage) Retry(ctx context.Context, p authz.Principal) (int, error) {
	var n int
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetClaimStorage(ctx)
		if database.IsNoRows(err) || (err == nil && row.Provider == nil) {
			return apperr.New(api.ErrorCodeCLAIMSTORAGENOTCONNECTED, "Connect Google Drive, OneDrive or Dropbox first.")
		}
		if err != nil {
			return err
		}
		if row.Status == "reconnect" {
			return apperr.New(api.ErrorCodeCLAIMSTORAGENOTCONNECTED, ProviderName(*row.Provider)+" needs connecting again first.")
		}
		n, err = s.uploadAgain(ctx, tx, p.TenantID)
		return err
	})
	return n, err
}

// uploadAgain queues every unfinished upload again (everyone's claims).
func (s *Storage) uploadAgain(ctx context.Context, tx pgx.Tx, tenant string) (int, error) {
	n := 0
	err := asSystem(ctx, tx, func(q *store.Queries) error {
		ids, err := q.ClaimsToUploadAgain(ctx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := s.queue(ctx, tx, q, tenant, id); err != nil {
				return err
			}
		}
		n = len(ids)
		return nil
	})
	return n, err
}

func (s *Storage) queue(ctx context.Context, tx pgx.Tx, q *store.Queries, tenant, id string) error {
	seq, err := q.QueueClaimUpload(ctx, id)
	if err != nil {
		return err
	}
	_, err = jobqueue.Enqueue(ctx, tx, tenant, api.JobKindClaimUpload, api.ClaimUploadJob{ClaimId: id, Sequence: int(seq)})
	return err
}

// Submitted queues the upload of claims just submitted, in the submit's
// transaction, when an account is connected.
func (s *Storage) Submitted(ctx context.Context, tx pgx.Tx, tenant string, ids []string) error {
	return asSystem(ctx, tx, func(q *store.Queries) error {
		row, err := q.GetClaimStorage(ctx)
		if database.IsNoRows(err) || (err == nil && row.Provider == nil) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := s.queue(ctx, tx, q, tenant, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// Deleted queues the removal of a deleted claim's uploaded receipt.
func (s *Storage) Deleted(ctx context.Context, tx pgx.Tx, tenant string, row store.DeleteClaimRow) error {
	return removeLater(ctx, tx, tenant, row.UploadProvider, row.UploadAccount, row.UploadRemoteID)
}

// Withdrawn lets go of a withdrawn claim's upload: its receipt leaves the
// drive (queued), and an upload still on its way leaves it be.
func (s *Storage) Withdrawn(ctx context.Context, tx pgx.Tx, tenant string, row store.Claim) error {
	if row.UploadStatus == "none" && row.UploadRemoteID == nil {
		return nil
	}
	return asSystem(ctx, tx, func(q *store.Queries) error {
		if err := q.ClearClaimUpload(ctx, row.ID); err != nil {
			return err
		}
		return removeLater(ctx, tx, tenant, row.UploadProvider, row.UploadAccount, row.UploadRemoteID)
	})
}

func removeLater(ctx context.Context, tx pgx.Tx, tenant string, provider, account, remoteID *string) error {
	if remoteID == nil || provider == nil || account == nil {
		return nil
	}
	_, err := jobqueue.Enqueue(ctx, tx, tenant, api.JobKindClaimRemoveUpload, api.ClaimRemoveUploadJob{
		Provider: api.ClaimStorageProvider(*provider), Account: *account, RemoteId: *remoteID,
	})
	return err
}
