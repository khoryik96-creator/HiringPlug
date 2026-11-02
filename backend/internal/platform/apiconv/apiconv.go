// Package apiconv converts between contract types (generated, with nullable
// fields) and plain Go values, for module HTTP handlers. Partial updates
// (PATCH) use Change: absent leaves a field alone, null clears it, a value
// sets it.
package apiconv

import "github.com/oapi-codegen/nullable"

// Change is one field of a partial update.
type Change[T any] struct {
	Set   bool
	Value T
}

// Apply replaces *field when the change is set.
func Apply[T any](field *T, c Change[T]) {
	if c.Set {
		*field = c.Value
	}
}

// Required converts a PATCH field that may be absent but not null.
func Required[T any](p *T) Change[T] {
	if p == nil {
		return Change[T]{}
	}
	return Change[T]{Set: true, Value: *p}
}

// Optional converts a nullable PATCH field: absent -> not set; null -> cleared;
// value -> set (converted with conv, e.g. an enum to string).
func Optional[T any, U any](n nullable.Nullable[T], conv func(T) U) Change[*U] {
	if !n.IsSpecified() {
		return Change[*U]{}
	}
	v, err := n.Get()
	if err != nil {
		return Change[*U]{Set: true, Value: nil}
	}
	u := conv(v)
	return Change[*U]{Set: true, Value: &u}
}

// OrNull returns JSON null for nil, otherwise the value. Use it for
// required-but-nullable response fields: an unspecified Nullable would
// serialize as the zero value instead of null.
func OrNull[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}

// ValueOrNil returns the value of a request field that may be absent or null.
func ValueOrNil[T any](n nullable.Nullable[T]) *T {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &v
}

// Convert converts between string-based types (enums), keeping nil.
func Convert[To ~string, From ~string](p *From) *To {
	if p == nil {
		return nil
	}
	v := To(*p)
	return &v
}

// Same is the identity conversion, for Optional on plain values.
func Same[T any](v T) T { return v }
