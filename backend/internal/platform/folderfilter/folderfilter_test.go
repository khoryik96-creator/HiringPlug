package folderfilter_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/folderfilter"
)

func TestAListShowsEverythingOrOnlyAFoldersRecords(t *testing.T) {
	ctx := context.Background()
	in := folderfilter.Items(func(_ context.Context, _ pgx.Tx, _ authz.Principal, id string) ([]string, error) {
		return []string{"cand_" + id}, nil
	})
	ids, err := in.IDs(ctx, nil, authz.Principal{}, nil)
	require.NoError(t, err)
	assert.Nil(t, ids, "no folder: every record")
	folder := "x"
	ids, err = in.IDs(ctx, nil, authz.Principal{}, &folder)
	require.NoError(t, err)
	assert.Equal(t, []string{"cand_x"}, ids)

	var none folderfilter.Items // folders not wired in
	_, err = none.IDs(ctx, nil, authz.Principal{}, &folder)
	var appErr *apperr.Error
	require.ErrorAs(t, err, &appErr)
}
