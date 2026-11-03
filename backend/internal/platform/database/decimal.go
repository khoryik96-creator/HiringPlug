package database

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// DecimalString renders a numeric column exactly, e.g. "8500.00" (nil for
// NULL), so amounts never pass through floating point.
func DecimalString(n pgtype.Numeric) (*string, error) {
	if !n.Valid {
		return nil, nil //nolint:nilnil // a NULL column is not an error
	}
	v, err := n.Value()
	if err != nil {
		return nil, err
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("numeric value has unexpected type %T", v)
	}
	return &s, nil
}
