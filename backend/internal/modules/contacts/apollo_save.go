package contacts

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// saveApolloEmail puts the work email Apollo found for this person on their
// contact: the one active contact with this exact LinkedIn profile at this
// company. It goes through Update, so the address is
// checked, verified and audited like one typed in. An email the contact
// already has is replaced only when the person asked to replace that address.
func (s *Service) saveApolloEmail(ctx context.Context, p authz.Principal, companyID string, in api.ApolloLookupRequest) (api.ApolloLookup, error) {
	if !p.Can(api.PermissionContactsUpdate) {
		return api.ApolloLookup{}, apperr.New(api.ErrorCodeFORBIDDEN, "Your role may not change contacts, so it cannot save this email.")
	}
	found, err := s.lookupApollo(ctx, p, companyID, api.ApolloLookupRequest{Action: api.ApolloLookupRequestActionPreview, ProfileUrl: in.ProfileUrl})
	if err != nil || found.State == api.ApolloLookupStateExisting {
		return found, err // already on the contact
	}
	if found.State != api.ApolloLookupStateFound || found.Email == nil {
		return found, apperr.Validation(apperr.FieldErrors{{Field: "profileUrl", Message: "Apollo has not found a work email for this person"}})
	}
	var people []store.HRProfileContactsRow
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		people, err = store.New(tx).HRProfileContacts(ctx, store.HRProfileContactsParams{CompanyID: companyID, ProfileUrl: apolloIdentity(in.ProfileUrl)})
		return err
	})
	if err != nil {
		return found, err
	}
	switch {
	case len(people) == 0:
		return found, apperr.Validation(apperr.FieldErrors{{Field: "profileUrl", Message: "add this person as a contact at this company first"}})
	case len(people) > 1:
		return found, apperr.Validation(apperr.FieldErrors{{Field: "profileUrl", Message: "more than one contact has this LinkedIn profile; add the email on the right one"}})
	case people[0].Email != nil && *people[0].Email == *found.Email:
		return s.lookupApollo(ctx, p, companyID, api.ApolloLookupRequest{Action: api.ApolloLookupRequestActionPreview, ProfileUrl: in.ProfileUrl})
	case people[0].Email != nil && !strings.EqualFold(deref(in.Replace), *people[0].Email):
		// Never overwrite without asking: say what the contact has, so the
		// person can choose to replace exactly that address.
		found.ContactEmail = people[0].Email
		return found, nil
	}
	if _, err := s.Update(ctx, p, people[0].ID, int(people[0].Version), Patch{Email: apiconv.Change[*string]{Set: true, Value: found.Email}}); err != nil {
		return found, err
	}
	return s.lookupApollo(ctx, p, companyID, api.ApolloLookupRequest{Action: api.ApolloLookupRequestActionPreview, ProfileUrl: in.ProfileUrl})
}
