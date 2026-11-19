package emails

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
)

func TestOutlookRuleFolderPathDistinguishesFoldersWithTheSameName(t *testing.T) {
	first, second := "first", "second"
	folders := map[string]store.EmailOutlookFolder{
		first:  {RemoteID: first, Name: "Operations"},
		second: {RemoteID: second, Name: "Recruiting"},
	}
	require.Equal(t, "Operations / Applicants", outlookRuleFolderPath(store.EmailOutlookFolder{
		RemoteID: "ops-applicants", Name: "Applicants", ParentID: &first,
	}, folders))
	require.Equal(t, "Recruiting / Applicants", outlookRuleFolderPath(store.EmailOutlookFolder{
		RemoteID: "rec-applicants", Name: "Applicants", ParentID: &second,
	}, folders))
}
