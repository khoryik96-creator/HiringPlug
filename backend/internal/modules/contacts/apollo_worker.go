package contacts

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// OutsideJobHandlers performs the slow broker call after the claim commits.
func (m *Module) OutsideJobHandlers() jobqueue.OutsideHandlers {
	return jobqueue.OutsideHandlers{api.JobKindContactApolloLookup: m.service.runApolloLookup}
}

func (s *Service) runApolloLookup(ctx context.Context, job jobqueue.Job) error {
	var in api.ContactApolloLookupJob
	if err := job.Decode(&in); err != nil {
		return err
	}
	p := authz.System(job.TenantID)
	var work apolloWork
	var subject apolloPerson
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.ClaimApolloLookup(ctx, in.LookupId)
		if database.IsNoRows(err) {
			// Expiry is conditional on being unclaimed. A job replay never
			// sends an operation that was already claimed.
			_, err = expireQueuedApolloLookup(ctx, tx, in.LookupId)
			return err
		}
		if err != nil {
			return err
		}
		work.lookup = row
		allowed, err := s.apolloUserAllowed(ctx, tx, row.UserID)
		if err != nil || !allowed || time.Since(row.CreatedAt) > 15*time.Minute {
			return err
		}
		work.connection, err = q.LockApolloConnectionForLookup(ctx, row.UserID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil || work.connection.FlowID != row.FlowID ||
			s.apolloConnection(work.connection).State != api.ApolloConnectionStateConnected {
			return err
		}
		current, readiness, err := s.apolloSubject(ctx, tx, row.CompanyID, row.ProfileUrl)
		if problem, ok := apperr.As(err); ok && problem.Code == api.ErrorCodeNOTFOUND {
			return nil
		}
		if err != nil {
			return err
		}
		expected := apolloPerson{URL: row.ProfileUrl, Name: row.PersonName, Company: row.LinkedinCompanyName,
			CompanyID: row.LinkedinCompanyID, CompanyURL: row.LinkedinCompanyUrl}
		if readiness != api.ApolloLookupStateReady || current != expected {
			return nil
		}
		existing, err := s.existingHRWorkEmail(ctx, q, row.CompanyID, row.ProfileUrl)
		if err == nil && existing == "" {
			subject = expected
		}
		return err
	})
	if err != nil || work.lookup.ID == "" {
		return err
	}

	result := api.ApolloLookup{State: api.ApolloLookupStateCancelled}
	if subject.URL != "" {
		if proof, proofErr := s.apolloProof(work.connection); proofErr == nil {
			auth := apolloLookupAuth(work, proof)
			reply, status, _ := s.apollo.post(ctx, "/reveal-v2", map[string]any{
				"connection_id": auth["connection_id"], "secret": proof, "operation_id": auth["operation_id"], "confirmed": true, "person": subject,
			})
			result = normalizeApolloReply(reply, status)
		}
	}
	// Record the outcome even if the worker's context expired during the call.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _, err = s.saveApolloResult(saveCtx, p, work, result)
	return err
}

// apolloUserAllowed reports whether the recruiter who confirmed a lookup is
// still active and may still use Apollo: a role changed after the
// confirmation stops the paid call.
func (s *Service) apolloUserAllowed(ctx context.Context, tx pgx.Tx, user string) (bool, error) {
	if s.apolloUsers == nil {
		return false, nil
	}
	active, err := s.apolloUsers.ActiveUsers(ctx, tx, []string{user})
	if err != nil {
		return false, err
	}
	perms, ok := active[user]
	return ok && slices.Contains(perms, api.PermissionContactsApolloUse), nil
}

// The reasons a broker gives with a limited lookup: Apollo's refusal.
var apolloRefusals = map[api.ApolloLookupReason]bool{
	api.ApolloLookupReasonNoCredits:      true,
	api.ApolloLookupReasonPlanNotAllowed: true,
	api.ApolloLookupReasonRateLimited:    true,
}

func normalizeApolloReply(reply apolloReply, status int) api.ApolloLookup {
	out := api.ApolloLookup{State: api.ApolloLookupStateUnconfirmed}
	if reply.NotDispatched {
		switch api.ApolloLookupState(reply.State) {
		case api.ApolloLookupStateLimited:
			return api.ApolloLookup{State: api.ApolloLookupStateCancelled, Reason: ptr(api.ApolloLookupReasonRetryableLimit)}
		case api.ApolloLookupStateAuthRequired:
			return api.ApolloLookup{State: api.ApolloLookupStateCancelled, Reason: ptr(api.ApolloLookupReasonReconnectRequired)}
		default:
			return out
		}
	}
	if status == http.StatusUnauthorized {
		out.State = api.ApolloLookupStateAuthRequired
		return out
	}
	switch api.ApolloLookupState(reply.State) {
	case api.ApolloLookupStateFound:
		if !reply.IdentityChecked {
			return out
		}
		if reason := apolloEmailReject(reply.Email); reason != "" {
			return api.ApolloLookup{State: api.ApolloLookupStateNotFound, Reason: &reason}
		}
		emailStatus := api.ApolloLookupEmailStatus(reply.EmailStatus)
		if !emailStatus.Valid() {
			emailStatus = api.ApolloLookupEmailStatusUnknown
		}
		return api.ApolloLookup{State: api.ApolloLookupStateFound, Email: &reply.Email, EmailStatus: &emailStatus}
	case api.ApolloLookupStateNotFound:
		out.State = api.ApolloLookupStateNotFound
		if reason := api.ApolloLookupReason(reply.Reason); reason.Valid() {
			out.Reason = &reason
		}
	case api.ApolloLookupStateLimited:
		out.State = api.ApolloLookupStateLimited
		// Which refusal Apollo gave (no credits, plan, too many requests).
		if reason := api.ApolloLookupReason(reply.Reason); apolloRefusals[reason] {
			out.Reason = &reason
		}
	case api.ApolloLookupStateMismatch, api.ApolloLookupStatePending, api.ApolloLookupStateAuthRequired:
		out.State = api.ApolloLookupState(reply.State)
	default:
		// All other states are local or indicate an unexpected broker response.
	}
	return out
}

func (s *Service) saveApolloResult(ctx context.Context, p authz.Principal, w apolloWork, result api.ApolloLookup) (store.HrApolloLookup, store.HrApolloConnection, error) {
	var row store.HrApolloLookup
	var connection store.HrApolloConnection
	in := store.FinishApolloLookupParams{ID: w.lookup.ID, State: string(result.State)}
	if result.Email != nil {
		sealed, err := s.apollo.seal(w.lookup.TenantID, w.lookup.UserID, w.connection.BrokerOrigin, w.lookup.ID, "email", *result.Email)
		if err != nil {
			return row, connection, err
		}
		in.EmailSealed = &sealed
	}
	if result.EmailStatus != nil {
		in.EmailStatus = ptr(string(*result.EmailStatus))
	}
	if result.Reason != nil {
		in.Reason = ptr(string(*result.Reason))
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		row, err = q.FinishApolloLookup(ctx, in)
		if database.IsNoRows(err) {
			row, err = q.GetApolloLookup(ctx, in.ID)
			if database.IsNoRows(err) { // company or user was removed during the lookup
				return nil
			}
		} else if err == nil {
			err = audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_lookup_outcome", EntityType: "company", EntityID: w.lookup.CompanyID,
				Changes: map[string]any{"lookupId": in.ID, "state": in.State}})
		}
		if err != nil {
			return err
		}
		connection, err = getApolloRow(ctx, q, row.UserID)
		return err
	})
	return row, connection, err
}

func expireQueuedApolloLookup(ctx context.Context, tx pgx.Tx, id string) (store.HrApolloLookup, error) {
	row, err := store.New(tx).ExpireQueuedApolloLookup(ctx, id)
	if database.IsNoRows(err) {
		return row, nil
	}
	if err != nil {
		return row, err
	}
	return row, audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_lookup_outcome", EntityType: "company", EntityID: row.CompanyID,
		Changes: map[string]any{"lookupId": id, "state": row.State, "reason": "queue_expired"}})
}
