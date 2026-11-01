package api

import (
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

// Spec returns the bundled API contract embedded in this binary, parsed once.
// (This file is hand-written; api.gen.go next to it is generated.)
var Spec = sync.OnceValues(func() (*openapi3.T, error) { return GetSpec() })
