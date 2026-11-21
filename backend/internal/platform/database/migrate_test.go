package database

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
)

func TestApolloPreviewUpgradePermitsOnlyTheLeadsGap(t *testing.T) {
	t.Parallel()
	migration := func(version int64, file string, state goose.State) *goose.MigrationStatus {
		return &goose.MigrationStatus{Source: &goose.Source{Version: version, Path: file}, State: state}
	}

	leads := migration(156, "00156_leads_workspace.sql", goose.StatePending)
	apollo := migration(157, "00157_apollo_hr.sql", goose.StateApplied)
	revocation := migration(158, "00158_apollo_revocation.sql", goose.StatePending)
	assert.True(t, isApolloPreviewUpgrade([]*goose.MigrationStatus{leads, apollo, revocation}))
	assert.False(t, isApolloPreviewUpgrade([]*goose.MigrationStatus{apollo, revocation}))
	assert.False(t, isApolloPreviewUpgrade([]*goose.MigrationStatus{leads, revocation}))

	gap := migration(155, "00155_email_delivery_attempt.sql", goose.StatePending)
	assert.False(t, isApolloPreviewUpgrade([]*goose.MigrationStatus{gap, leads, apollo, revocation}))
	wrong := migration(156, "00156_unrelated.sql", goose.StatePending)
	assert.False(t, isApolloPreviewUpgrade([]*goose.MigrationStatus{wrong, apollo, revocation}))
}
