package app

// This file is the one place modules are wired together. To add a module
// (`task new:module -- name` does all of this for you):
//  1. add a field to Modules and construct it in NewModules;
//  2. embed its HTTP handlers in Server (if it has endpoints);
//  3. add its JobHandlers in JobHandlers (if it handles jobs).
// The compiler then checks that every endpoint in the contract has a handler,
// and the worker refuses to start if a job kind has none.

import (
	"fmt"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	// scaffold:imports
)

// Modules holds every module, wired together.
type Modules struct {
	Identity   *identity.Module
	Candidates *candidates.Module
	Documents  *documents.Module
	Companies  *companies.Module
	// scaffold:fields
}

// NewModules builds every module. A module may receive another module (or a
// small interface it satisfies) to call its public API, as Documents does
// with Candidates.
func NewModules(d Deps) *Modules {
	m := &Modules{}
	m.Identity = identity.New(d.Pool, d.Verifier)
	m.Candidates = candidates.New(d.Pool, d.Config.DefaultPhoneArea)
	m.Documents = documents.New(d.Pool, d.Files, m.Candidates, d.Config.MaxUploadBytes)
	m.Companies = companies.New(d.Pool)
	// scaffold:constructors
	return m
}

// Each module's handler type is called HTTP; these aliases give the embedded
// fields distinct names.
type (
	identityHTTP   = identity.HTTP
	candidatesHTTP = candidates.HTTP
	documentsHTTP  = documents.HTTP
	companiesHTTP  = companies.HTTP
	// scaffold:aliases
)

// Server combines every module's handlers into the single interface the
// contract defines. If an endpoint in contracts/ has no handler, or two
// modules implement the same one, this file does not compile.
type Server struct {
	*identityHTTP
	*candidatesHTTP
	*documentsHTTP
	*companiesHTTP
	// scaffold:embeds
}

var _ api.StrictServerInterface = (*Server)(nil)

// Server returns the composed HTTP handlers.
func (m *Modules) Server() *Server {
	return &Server{
		identityHTTP:   m.Identity.HTTP,
		candidatesHTTP: m.Candidates.HTTP,
		documentsHTTP:  m.Documents.HTTP,
		companiesHTTP:  m.Companies.HTTP,
		// scaffold:server-fields
	}
}

// JobHandlers merges every module's handlers for the core queue.
func (m *Modules) JobHandlers() (jobqueue.Handlers, error) {
	all := jobqueue.Handlers{}
	for _, handlers := range []jobqueue.Handlers{
		m.Documents.JobHandlers(),
		// scaffold:job-handlers
	} {
		for kind, h := range handlers {
			if _, dup := all[kind]; dup {
				return nil, fmt.Errorf("two modules handle job kind %q", kind)
			}
			all[kind] = h
		}
	}
	return all, nil
}
