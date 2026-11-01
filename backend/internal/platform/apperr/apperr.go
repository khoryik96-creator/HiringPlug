// Package apperr is the one error type the API returns to clients.
//
// Every error has a stable Code from the contract's ErrorCode enum. The HTTP
// status for each code is declared once, in the contract (x-error-codes), and
// read from the embedded spec here, so Go, the web app and the docs agree.
// Anything that is not an *Error becomes a 500 INTERNAL, logged with its cause
// but never shown to the client.
package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Error is an expected failure with a stable code and a safe message.
type Error struct {
	Code    api.ErrorCode
	Message string
	Details map[string]any
	cause   error // for logs only; never sent to clients
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// New creates an error with a code and a message safe to show to users.
func New(code api.ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithDetail adds machine-readable context, e.g. existingCandidateId.
func (e *Error) WithDetail(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
	return e
}

// WithCause attaches the underlying error for logs.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// FieldError describes one invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// FieldErrors collects validation problems so all of them are reported at once.
type FieldErrors []FieldError

// Add records a problem with field.
func (f *FieldErrors) Add(field, message string) {
	*f = append(*f, FieldError{Field: field, Message: message})
}

// Err returns a VALIDATION_FAILED error listing every field, or nil if none.
func (f FieldErrors) Err() error {
	if len(f) == 0 {
		return nil
	}
	return Validation(f)
}

// Validation returns VALIDATION_FAILED with details.fields.
func Validation(fields FieldErrors) *Error {
	return New(api.ErrorCodeVALIDATIONFAILED, "Some fields are invalid.").WithDetail("fields", fields)
}

// NotFound returns NOT_FOUND for a kind of record ("candidate", "document").
func NotFound(what string) *Error {
	return New(api.ErrorCodeNOTFOUND, fmt.Sprintf("That %s does not exist or you cannot see it.", what))
}

// StaleVersion returns CONFLICT_STALE_VERSION.
func StaleVersion(what string) *Error {
	return New(api.ErrorCodeCONFLICTSTALEVERSION, fmt.Sprintf("This %s was changed by someone else. Reload it and try again.", what))
}

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// Status returns the HTTP status for code, as declared in the contract.
func Status(code api.ErrorCode) int {
	if s, ok := statuses()[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

var statuses = sync.OnceValue(func() map[api.ErrorCode]int {
	spec, err := api.Spec()
	if err != nil {
		panic(fmt.Sprintf("apperr: embedded API contract does not load: %v", err))
	}
	raw, err := json.Marshal(spec.Extensions["x-error-codes"])
	if err != nil {
		panic(fmt.Sprintf("apperr: x-error-codes: %v", err))
	}
	var codes map[string]struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(raw, &codes); err != nil {
		panic(fmt.Sprintf("apperr: x-error-codes: %v", err))
	}
	out := make(map[api.ErrorCode]int, len(codes))
	for code, c := range codes {
		out[api.ErrorCode(code)] = c.Status
	}
	return out
})
