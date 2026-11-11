// Package classification defines shared, transport-independent selection values.
package classification

// Selections keeps stable category IDs with the catalogue version the editor reviewed.
type Selections struct {
	CatalogueVersion int
	Ids              []string
	PrimaryId        *string
}
