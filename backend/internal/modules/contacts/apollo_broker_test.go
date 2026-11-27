package contacts

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
)

func TestApolloAcceptsBusinessDomainsThatSharePersonalProviderPrefixes(t *testing.T) {
	t.Parallel()
	for _, email := range []string{"hr@proton.com", "hr@mail.example.com", "hr@live.example.com"} {
		t.Run(email, func(t *testing.T) {
			t.Parallel()
			result := normalizeApolloReply(apolloReply{State: "found", IdentityChecked: true, Email: email, EmailStatus: "new-apollo-status"}, http.StatusOK)
			assert.Equal(t, api.ApolloLookupStateFound, result.State)
			assert.Equal(t, ptr(api.ApolloLookupEmailStatusUnknown), result.EmailStatus)
		})
	}
	for email, reason := range map[string]api.ApolloLookupReason{
		"a@Proton.Me": api.ApolloLookupReasonPersonalEmailOnly, "a@yahoo.com.my": api.ApolloLookupReasonPersonalEmailOnly,
		"email_not_unlocked@domain.com": api.ApolloLookupReasonEmailRedacted, "A Person <a@work.com>": api.ApolloLookupReasonEmailUnusable,
	} {
		t.Run(email, func(t *testing.T) {
			t.Parallel()
			result := normalizeApolloReply(apolloReply{State: "found", IdentityChecked: true, Email: email}, http.StatusOK)
			assert.Equal(t, api.ApolloLookupStateNotFound, result.State)
			assert.Equal(t, &reason, result.Reason)
			assert.Nil(t, result.Email)
		})
	}
}

func TestApolloRejectsUnexpectedOAuthDestinationStateAndCallback(t *testing.T) {
	t.Parallel()
	origin := "https://apollo.example.workers.dev"
	client := apolloClient{Apollo: config.Apollo{BrokerOrigin: origin}}
	id := strings.Repeat("ab", 32)
	link := "https://app.apollo.io/#/oauth/authorize?client_id=test&state=" + id +
		"&redirect_uri=https%3A%2F%2Fapollo.example.workers.dev%2Fcallback&response_type=code&scope=read_user_profile+app_scopes+people_match"
	assert.True(t, client.validLogin(apolloReply{FlowID: id, AuthorizationURL: link}))
	for _, bad := range []string{
		strings.Replace(link, "app.apollo.io", "app.apollo.io.evil.example", 1),
		strings.Replace(link, "app.apollo.io", "wrong-user@app.apollo.io", 1),
		strings.Replace(link, "callback", "something-else", 1),
		strings.Replace(link, id, strings.Repeat("b", 64), 1),
		link + "&state=" + id,
	} {
		assert.False(t, client.validLogin(apolloReply{FlowID: id, AuthorizationURL: bad}), bad)
	}
}

func TestApolloRequiresBrokerIdentityVerification(t *testing.T) {
	t.Parallel()
	result := normalizeApolloReply(apolloReply{State: "found", Email: "hr@work.example"}, http.StatusOK)
	assert.Equal(t, api.ApolloLookupStateUnconfirmed, result.State)
	assert.Nil(t, result.Email)
	result = normalizeApolloReply(apolloReply{State: "limited"}, http.StatusOK)
	assert.Equal(t, api.ApolloLookupStateLimited, result.State)
	assert.Nil(t, result.Reason)
	result = normalizeApolloReply(apolloReply{State: "limited", Reason: "no_credits"}, http.StatusOK)
	assert.Equal(t, api.ApolloLookupStateLimited, result.State)
	assert.Equal(t, api.ApolloLookupReasonNoCredits, *result.Reason, "which refusal Apollo gave is kept")
	result = normalizeApolloReply(apolloReply{State: "limited", Reason: "profile_not_found"}, http.StatusOK)
	assert.Nil(t, result.Reason, "a reason that does not belong to a refusal is dropped")
	result = normalizeApolloReply(apolloReply{State: "limited", NotDispatched: true}, http.StatusOK)
	assert.Equal(t, api.ApolloLookupReasonRetryableLimit, *result.Reason)
	assert.Equal(t, api.ApolloLookupStateCancelled, result.State)
}
