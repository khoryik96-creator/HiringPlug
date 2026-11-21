package contacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func (s *Service) apolloConnection(row store.HrApolloConnection) api.ApolloConnection {
	switch {
	case row.Revoking:
		return api.ApolloConnection{State: api.ApolloConnectionStateDisconnectedLocally}
	case !s.apollo.enabled():
		return api.ApolloConnection{State: api.ApolloConnectionStateDisabled}
	case row.FlowID == "":
		return api.ApolloConnection{State: api.ApolloConnectionStateDisconnected}
	case row.BrokerOrigin != s.apollo.BrokerOrigin || !row.ExpiresAt.After(time.Now()):
		return api.ApolloConnection{State: api.ApolloConnectionStateExpired}
	}
	// A changed or lost encryption key also requires a fresh connection.
	if _, err := s.apolloProof(row); err != nil {
		return api.ApolloConnection{State: api.ApolloConnectionStateExpired}
	}
	if row.AccountName != nil {
		return api.ApolloConnection{State: api.ApolloConnectionStateConnected, AccountName: row.AccountName}
	}
	if !s.apollo.validLogin(apolloReply{FlowID: row.FlowID, AuthorizationURL: row.AuthorizeUrl}) {
		return api.ApolloConnection{State: api.ApolloConnectionStateExpired}
	}
	return api.ApolloConnection{State: api.ApolloConnectionStatePending, AuthorizationUrl: &row.AuthorizeUrl}
}

func (s *Service) apolloProof(row store.HrApolloConnection) (string, error) {
	if row.BrokerOrigin != s.apollo.BrokerOrigin {
		return "", errApolloResponse
	}
	return s.apollo.open(row.TenantID, row.UserID, row.BrokerOrigin, row.FlowID, "proof", row.ProofSealed)
}

func getApolloRow(ctx context.Context, q *store.Queries, user string) (store.HrApolloConnection, error) {
	row, err := q.GetApolloConnection(ctx, user)
	if database.IsNoRows(err) {
		return row, nil
	}
	return row, err
}

func (s *Service) readApolloConnection(ctx context.Context, p authz.Principal) (store.HrApolloConnection, error) {
	var row store.HrApolloConnection
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		row, err = getApolloRow(ctx, store.New(tx), p.UserID)
		return err
	})
	return row, err
}

func (s *Service) startApollo(ctx context.Context, p authz.Principal) (api.ApolloConnection, error) {
	out := api.ApolloConnection{State: api.ApolloConnectionStateDisabled}
	if !s.apollo.enabled() {
		return out, nil
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockApolloAccount(ctx); err != nil {
			return err
		}
		row, err := getApolloRow(ctx, q, p.UserID)
		if err != nil {
			return err
		}
		out = s.apolloConnection(row)
		if row.FlowID != "" {
			return nil
		}

		// Serialize starts from multiple tabs. The free handshake is bounded;
		// the worker's paid lookup never holds a database transaction open.
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		secret := apolloRandomID()
		hash := sha256.Sum256([]byte(secret))
		reply, status, err := s.apollo.post(requestCtx, "/start", map[string]string{"proof_hash": hex.EncodeToString(hash[:])})
		if err != nil || reply.Protocol != apolloProtocol || !s.apollo.validLogin(reply) {
			return apolloConnectionError(status)
		}
		sealed, err := s.apollo.seal(p.TenantID, p.UserID, s.apollo.BrokerOrigin, reply.FlowID, "proof", secret)
		if err != nil {
			return err
		}
		if err := q.SetApolloFlow(ctx, store.SetApolloFlowParams{FlowID: reply.FlowID, BrokerOrigin: s.apollo.BrokerOrigin,
			ProofSealed: sealed, AuthorizeUrl: reply.AuthorizationURL}); err != nil {
			return err
		}
		out = api.ApolloConnection{State: api.ApolloConnectionStatePending, AuthorizationUrl: &reply.AuthorizationURL}
		return audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_signin_started", EntityType: "user", EntityID: p.UserID})
	})
	return out, err
}

func (s *Service) pollApollo(ctx context.Context, p authz.Principal) (api.ApolloConnection, error) {
	row, err := s.readApolloConnection(ctx, p)
	if err != nil {
		return api.ApolloConnection{}, err
	}
	out := s.apolloConnection(row)
	if out.State != api.ApolloConnectionStatePending {
		return out, nil
	}
	proof, err := s.apolloProof(row)
	if err != nil {
		return out, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reply, status, err := s.apollo.post(requestCtx, "/poll", map[string]string{"flow_id": row.FlowID, "secret": proof})
	switch {
	case status == http.StatusUnauthorized:
		out = api.ApolloConnection{State: api.ApolloConnectionStateExpired}
	case err != nil:
		return out, apolloConnectionError(status)
	case reply.State == "pending":
		return out, nil
	case reply.State == "failed":
		out = api.ApolloConnection{State: api.ApolloConnectionStateFailed}
	case reply.State == "connected" && reply.ConnectionID == row.FlowID:
		name := strings.TrimSpace(reply.Name)
		if name == "" || len(name) > 200 || strings.IndexFunc(name, apolloControl) >= 0 {
			name = "Apollo account"
		}
		err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
			q := store.New(tx)
			saved, err := q.FinishApolloFlow(ctx, store.FinishApolloFlowParams{FlowID: row.FlowID, AccountName: &name})
			if database.IsNoRows(err) { // another tab finished or disconnected it
				saved, err = getApolloRow(ctx, q, p.UserID)
				out = s.apolloConnection(saved)
				return err
			}
			if err != nil {
				return err
			}
			out = s.apolloConnection(saved)
			return audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_connected", EntityType: "user", EntityID: p.UserID})
		})
		return out, err
	default:
		return out, apolloConnectionError(http.StatusOK)
	}

	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).ExpireApolloFlow(ctx, row.FlowID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_signin_failed", EntityType: "user", EntityID: p.UserID})
	})
	return out, err
}

func (s *Service) disconnectApollo(ctx context.Context, p authz.Principal) (api.ApolloConnection, error) {
	out := api.ApolloConnection{State: api.ApolloConnectionStateDisconnected}
	var row store.HrApolloConnection
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockApolloAccount(ctx); err != nil {
			return err
		}
		var err error
		row, err = getApolloRow(ctx, q, p.UserID)
		if err != nil || row.FlowID == "" || row.Revoking {
			return err
		}
		if err := q.MarkApolloRevoking(ctx, row.FlowID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_revocation_started", EntityType: "user", EntityID: p.UserID})
	})
	if err != nil || row.FlowID == "" {
		return out, err
	}

	// Keep the sealed proof until the broker acknowledges revocation. Lookups
	// are blocked locally immediately, including on an expired OAuth flow.
	out.State = api.ApolloConnectionStateDisconnectedLocally
	proof, err := s.apolloProof(row)
	if err != nil || !s.apollo.enabled() {
		return out, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reply, _, err := s.apollo.post(requestCtx, "/disconnect-v2", map[string]string{"connection_id": row.FlowID, "secret": proof})
	if err != nil || reply.Protocol != apolloProtocol || reply.Connected == nil || *reply.Connected {
		return out, nil
	}
	return s.finishApolloDisconnect(ctx, p, row.FlowID, "contacts.apollo_disconnected")
}

func (s *Service) finishApolloDisconnect(ctx context.Context, p authz.Principal, flow, action string) (api.ApolloConnection, error) {
	var out api.ApolloConnection
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockApolloAccount(ctx); err != nil {
			return err
		}
		row, err := getApolloRow(ctx, q, p.UserID)
		if err != nil {
			return err
		}
		if row.Revoking && row.FlowID == flow {
			if err := q.DeleteApolloConnection(ctx, flow); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "user", EntityID: p.UserID}); err != nil {
				return err
			}
			row = store.HrApolloConnection{}
		}
		out = s.apolloConnection(row)
		return nil
	})
	return out, err
}

func (s *Service) forgetApollo(ctx context.Context, p authz.Principal) (api.ApolloConnection, error) {
	row, err := s.readApolloConnection(ctx, p)
	if err != nil {
		return api.ApolloConnection{}, err
	}
	return s.finishApolloDisconnect(ctx, p, row.FlowID, "contacts.apollo_connection_forgotten")
}
