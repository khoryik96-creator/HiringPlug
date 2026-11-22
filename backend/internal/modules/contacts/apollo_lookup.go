package contacts

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

const apolloSendingGrace = time.Minute

func apolloControl(r rune) bool { return unicode.IsControl(r) }

func apolloIdentity(profile string) string {
	return strings.ToLower(ProfileURL(profile))
}

func (s *Service) apolloSubject(ctx context.Context, tx pgx.Tx, companyID, profile string) (apolloPerson, api.ApolloLookupState, error) {
	if err := s.hrCompany(ctx, tx, companyID); err != nil {
		return apolloPerson{}, "", err
	}
	scan, err := store.New(tx).GetHrScan(ctx, companyID)
	if database.IsNoRows(err) {
		return apolloPerson{}, "", apperr.NotFound("Find HR scan")
	}
	if err != nil {
		return apolloPerson{}, "", err
	}
	var people []HRPerson
	if err := json.Unmarshal(scan.People, &people); err != nil {
		return apolloPerson{}, "", err
	}
	for _, p := range people {
		if apolloIdentity(p.URL) != profile {
			continue
		}
		if p.Current != nil && !*p.Current || strings.TrimSpace(p.Name) == "" ||
			len(p.Name) > 200 || len(scan.LinkedinCompanyName) > 200 ||
			strings.IndexFunc(p.Name+scan.LinkedinCompanyName, apolloControl) >= 0 {
			return apolloPerson{}, api.ApolloLookupStateIneligible, nil
		}
		current, err := s.companies.LinkedinURL(ctx, tx, companyID)
		if err != nil {
			return apolloPerson{}, "", err
		}
		subject := apolloPerson{URL: profile, Name: p.Name, Company: scan.LinkedinCompanyName,
			CompanyID: scan.LinkedinCompanyID, CompanyURL: deref(scan.LinkedinCompanyUrl)}
		if subject.CompanyURL == "" || subject.CompanyURL != apolloCompanyURL(current) {
			return subject, api.ApolloLookupStateRescan, nil
		}
		return subject, api.ApolloLookupStateReady, nil
	}
	return apolloPerson{}, "", apperr.NotFound("person in the Find HR scan")
}

type apolloWork struct {
	lookup     store.HrApolloLookup
	connection store.HrApolloConnection
}

func (s *Service) existingHRWorkEmail(ctx context.Context, q *store.Queries, company, profile string) (string, error) {
	emails, err := q.HRWorkEmail(ctx, store.HRWorkEmailParams{CompanyID: company, ProfileUrl: profile})
	if err != nil {
		return "", err
	}
	// A work address is reusable only when the exact profile at this company
	// resolves to one unambiguous contact with a work address.
	if len(emails) == 1 && apolloEmailReject(deref(emails[0])) == "" {
		return *emails[0], nil
	}
	return "", nil
}

func (s *Service) lookupApollo(ctx context.Context, p authz.Principal, companyID string, in api.ApolloLookupRequest) (api.ApolloLookup, error) {
	// The answer shows the Find HR scan and contacts' saved addresses, which
	// a role given Apollo without contacts:read may not see.
	if !p.Can(api.PermissionContactsRead) {
		return api.ApolloLookup{}, apperr.New(api.ErrorCodeFORBIDDEN, "Apollo lookup shows Find HR results and contacts' emails, which you may not see.")
	}
	profile := apolloIdentity(in.ProfileUrl)
	if profile == "" || !in.Action.Valid() {
		return api.ApolloLookup{}, apperr.Validation(apperr.FieldErrors{{Field: "profileUrl", Message: "choose a LinkedIn profile from this scan"}})
	}
	var out api.ApolloLookup
	var recoverWork apolloWork
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if in.Action == api.ApolloLookupRequestActionUnlock {
			if err := q.LockApolloAccount(ctx); err != nil {
				return err
			}
		}
		person, readiness, err := s.apolloSubject(ctx, tx, companyID, profile)
		if err != nil {
			return err
		}
		out.State = readiness
		if readiness == api.ApolloLookupStateIneligible {
			return nil
		}
		existing, err := s.existingHRWorkEmail(ctx, q, companyID, profile)
		if err != nil {
			return err
		}
		if existing != "" {
			out = api.ApolloLookup{State: api.ApolloLookupStateExisting, Email: &existing}
			return nil
		}
		if readiness != api.ApolloLookupStateReady {
			return nil
		}
		if !s.apollo.enabled() {
			out.State = api.ApolloLookupStateDisabled
			return nil
		}

		row, err := getApolloRow(ctx, q, p.UserID)
		if err != nil {
			return err
		}
		receipt, err := q.FindApolloLookup(ctx, store.FindApolloLookupParams{LinkedinCompanyID: person.CompanyID, ProfileUrl: profile})
		if err == nil && receipt.State == string(api.ApolloLookupStateQueued) {
			expired, expireErr := expireQueuedApolloLookup(ctx, tx, receipt.ID)
			if expireErr != nil {
				return expireErr
			}
			if expired.ID != "" {
				receipt = expired
			}
		}
		switch {
		case err == nil && receipt.State != string(api.ApolloLookupStateCancelled):
			out = s.apolloReceipt(receipt, row)
			if in.Action == api.ApolloLookupRequestActionCheck &&
				(out.State == api.ApolloLookupStatePending || out.State == api.ApolloLookupStateUnconfirmed) &&
				s.apolloConnection(row).State == api.ApolloConnectionStateConnected {
				recoverWork = apolloWork{lookup: receipt, connection: row}
			}
			return nil
		case err != nil && !database.IsNoRows(err):
			return err
		case s.apolloConnection(row).State != api.ApolloConnectionStateConnected:
			out.State = api.ApolloLookupStateDisconnected
			return nil
		}

		if receipt.ID != "" {
			out.State = api.ApolloLookupStateCancelled
			if receipt.Reason != nil {
				out.Reason = ptr(api.ApolloLookupReason(*receipt.Reason))
				if *out.Reason == api.ApolloLookupReasonReconnectRequired && receipt.FlowID == row.FlowID {
					return nil
				}
			}
		}
		busy, err := q.ApolloLookupInProgress(ctx, row.FlowID)
		if err != nil {
			return err
		}
		if busy {
			out.State = api.ApolloLookupStateBusy
			return nil
		}
		confirmation := s.apollo.confirmation(p.TenantID, p.UserID, companyID, row.FlowID, receipt.ID, person)
		if in.Action == api.ApolloLookupRequestActionPreview {
			out.Confirmation = &confirmation
			return nil
		}
		if in.Action != api.ApolloLookupRequestActionUnlock {
			return nil
		}
		if !hmac.Equal([]byte(confirmation), []byte(deref(in.Confirmation))) {
			return apperr.Validation(apperr.FieldErrors{{Field: "confirmation", Message: "review this person and confirm credit use again"}})
		}
		if receipt.ID != "" {
			// Only a lookup proven to have stopped before the broker call can be reset.
			if err := q.DeleteCancelledApolloLookup(ctx, receipt.ID); err != nil {
				return err
			}
		}
		receipt, err = q.CreateApolloLookup(ctx, store.CreateApolloLookupParams{ID: apolloRandomID(), FlowID: row.FlowID,
			CompanyID: companyID, LinkedinCompanyID: person.CompanyID, ProfileUrl: profile, PersonName: person.Name,
			LinkedinCompanyName: person.Company, LinkedinCompanyUrl: person.CompanyURL})
		if err != nil {
			return err
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindContactApolloLookup, api.ContactApolloLookupJob{LookupId: receipt.ID}); err != nil {
			return err
		}
		out.State = api.ApolloLookupStateQueued
		return audit.Record(ctx, tx, audit.Entry{Action: "contacts.apollo_lookup_requested", EntityType: "company", EntityID: companyID, Changes: map[string]any{"lookupId": receipt.ID}})
	})
	if err == nil && recoverWork.lookup.ID != "" {
		return s.checkApolloLookup(ctx, p, recoverWork)
	}
	return out, err
}

func (s *Service) apolloReceipt(receipt store.HrApolloLookup, conn store.HrApolloConnection) api.ApolloLookup {
	if receipt.ID == "" {
		return api.ApolloLookup{State: api.ApolloLookupStateIneligible}
	}
	if conn.Revoking || conn.FlowID != receipt.FlowID || conn.BrokerOrigin != s.apollo.BrokerOrigin {
		return api.ApolloLookup{State: api.ApolloLookupStatePreviousConnection}
	}
	out := api.ApolloLookup{State: api.ApolloLookupState(receipt.State)}
	if out.State == api.ApolloLookupStateSending && time.Since(receipt.UpdatedAt) >= apolloSendingGrace {
		out.State = api.ApolloLookupStateUnconfirmed
	}
	if receipt.Reason != nil {
		out.Reason = ptr(api.ApolloLookupReason(*receipt.Reason))
	}
	if out.State == api.ApolloLookupStateFound {
		email, err := s.apollo.open(receipt.TenantID, receipt.UserID, conn.BrokerOrigin, receipt.ID, "email", deref(receipt.EmailSealed))
		if err != nil {
			return api.ApolloLookup{State: api.ApolloLookupStatePreviousConnection}
		}
		out.Email, out.EmailStatus = &email, ptr(api.ApolloLookupEmailStatus(deref(receipt.EmailStatus)))
	}
	return out
}

func (s *Service) checkApolloLookup(ctx context.Context, p authz.Principal, w apolloWork) (api.ApolloLookup, error) {
	proof, err := s.apolloProof(w.connection)
	if err != nil {
		return api.ApolloLookup{State: api.ApolloLookupStatePreviousConnection}, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	reply, status, err := s.apollo.post(requestCtx, "/lookup-status-v2", apolloLookupAuth(w, proof))
	if err != nil && status != http.StatusUnauthorized {
		return api.ApolloLookup{State: api.ApolloLookupStateUnconfirmed}, nil
	}
	row, conn, err := s.saveApolloResult(ctx, p, w, normalizeApolloReply(reply, status))
	if err != nil {
		return api.ApolloLookup{}, err
	}
	return s.apolloReceipt(row, conn), nil
}

func apolloLookupAuth(w apolloWork, proof string) map[string]string {
	return map[string]string{"connection_id": w.connection.FlowID, "operation_id": w.lookup.ID, "secret": proof}
}
