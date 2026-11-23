package database

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
)

func migration(version int64, file string, state goose.State) *goose.MigrationStatus {
	return &goose.MigrationStatus{Source: &goose.Source{Version: version, Path: file}, State: state}
}

func TestOnlyKnownLateMigrationsMayBeAppliedOutOfOrder(t *testing.T) {
	t.Parallel()
	leads := migration(156, "00156_leads_workspace.sql", goose.StatePending)
	apollo := migration(157, "00157_apollo_hr.sql", goose.StateApplied)
	revocation := migration(158, "00158_apollo_revocation.sql", goose.StatePending)
	assert.True(t, lateButKnown([]*goose.MigrationStatus{leads, apollo, revocation}), "the Apollo preview's Leads gap")
	assert.False(t, lateButKnown([]*goose.MigrationStatus{apollo, revocation}), "no gap: nothing out of order")

	gap := migration(155, "00155_email_delivery_attempt.sql", goose.StatePending)
	assert.False(t, lateButKnown([]*goose.MigrationStatus{gap, leads, apollo, revocation}), "any other gap stops the update")
	wrong := migration(156, "00156_unrelated.sql", goose.StatePending)
	assert.False(t, lateButKnown([]*goose.MigrationStatus{wrong, apollo, revocation}), "only that file")
}

// A server that had 1.37.1 (up to 190) gets 171 from 1.37.2 late.
func TestTheClientLinkCascadesMayArriveAfter190(t *testing.T) {
	t.Parallel()
	cascades := migration(171, "00171_shortlist_cascades.sql", goose.StatePending)
	applied := []*goose.MigrationStatus{
		migration(170, "00170_client_shortlists.sql", goose.StateApplied), cascades,
		migration(180, "00180_activity_targets.sql", goose.StateApplied),
		migration(190, "00190_getting_started.sql", goose.StateApplied),
	}
	assert.True(t, lateButKnown(applied))
	newer := migration(191, "00191_something_new.sql", goose.StatePending)
	assert.True(t, lateButKnown(append(applied, newer)), "with newer ones to apply as well")
	unknown := migration(175, "00175_unknown.sql", goose.StatePending)
	assert.False(t, lateButKnown(append(applied, unknown)), "but not with an unknown gap")
}
