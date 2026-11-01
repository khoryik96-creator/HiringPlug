package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// Permission values with special meaning in x-permission.
const (
	permissionPublic        = "public"
	permissionAuthenticated = "authenticated"
)

// Authenticator turns a bearer token into the caller's principal.
type Authenticator func(ctx context.Context, bearerToken string) (authz.Principal, error)

// enforceContract runs before every API handler and applies the contract:
//  1. the method and path must exist in the contract;
//  2. the caller must be signed in and hold the operation's x-permission;
//  3. parameters and JSON body must match the contract's schemas.
//
// Handlers can therefore trust that their input is well-formed and allowed;
// they only check business rules.
func enforceContract(router routers.Router, authenticate Authenticator, maxBody, maxUpload int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, pathParams, err := router.FindRoute(r)
		if err != nil {
			if pathExistsForOtherMethod(router, r) {
				WriteError(w, r, apperr.New(api.ErrorCodeMETHODNOTALLOWED, "This endpoint does not support "+r.Method+"."))
			} else {
				WriteError(w, r, apperr.New(api.ErrorCodeROUTENOTFOUND, "No such endpoint."))
			}
			return
		}

		permission := operationPermission(route.Operation)
		if permission != permissionPublic {
			principal, err := authenticateRequest(r, authenticate)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			ctx := authz.WithPrincipal(r.Context(), principal)
			ctx = logx.With(ctx, "tenant_id", principal.TenantID, "user_id", principal.UserID)
			setCaller(ctx, principal.TenantID, principal.UserID)
			r = r.WithContext(ctx)
			if permission != permissionAuthenticated && !principal.Can(api.Permission(permission)) {
				WriteError(w, r, apperr.New(api.ErrorCodeFORBIDDEN, "You do not have permission to do this.").
					WithDetail("permission", permission))
				return
			}
		}

		multipart := strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
		limit := maxBody
		if multipart {
			limit = maxUpload + 1<<20 // room for the multipart framing around the file
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)

		input := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, // done above
				MultiError:         true,
				// Uploads are streamed and checked by the handler instead of
				// being buffered here.
				ExcludeRequestBody: multipart,
			},
		}
		if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
			WriteError(w, r, requestValidationError(err))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pathExistsForOtherMethod tells 405 (wrong method) apart from 404 (no such path).
func pathExistsForOtherMethod(router routers.Router, r *http.Request) bool {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if method == r.Method {
			continue
		}
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, _, err := router.FindRoute(probe); err == nil {
			return true
		}
	}
	return false
}

func operationPermission(op *openapi3.Operation) string {
	if v, ok := op.Extensions["x-permission"].(string); ok && v != "" {
		return v
	}
	// The contract bundler guarantees every operation has one; refuse rather
	// than accidentally serve an unprotected endpoint.
	panic(fmt.Sprintf("operation %s has no x-permission", op.OperationID))
}

func authenticateRequest(r *http.Request, authenticate Authenticator) (authz.Principal, error) {
	header := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return authz.Principal{}, apperr.New(api.ErrorCodeUNAUTHENTICATED, "Sign in first: send an Authorization: Bearer token.")
	}
	p, err := authenticate(r.Context(), strings.TrimSpace(token))
	if err != nil {
		if _, ok := apperr.As(err); ok {
			return authz.Principal{}, err
		}
		logx.From(r.Context()).Info("authentication failed", "reason", err.Error())
		return authz.Principal{}, apperr.New(api.ErrorCodeUNAUTHENTICATED, "Your sign-in is invalid or has expired. Sign in again.")
	}
	return p, nil
}

// requestValidationError turns the validator's errors into VALIDATION_FAILED
// (or 413/415) with one entry per invalid field.
func requestValidationError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return apperr.New(api.ErrorCodePAYLOADTOOLARGE, "The request is too large.")
	}
	var fields apperr.FieldErrors
	var unsupported bool
	collectRequestErrors(err, &fields, &unsupported)
	if unsupported {
		return apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "Send the request body as application/json.")
	}
	if len(fields) == 0 {
		fields.Add("request", err.Error())
	}
	return apperr.Validation(fields)
}

func collectRequestErrors(err error, fields *apperr.FieldErrors, unsupported *bool) {
	if multi, ok := err.(openapi3.MultiError); ok { //nolint:errorlint // MultiError is a slice type, not a wrapper
		for _, e := range multi {
			collectRequestErrors(e, fields, unsupported)
		}
		return
	}
	var reqErr *openapi3filter.RequestError
	if !errors.As(err, &reqErr) {
		fields.Add("request", err.Error())
		return
	}
	switch {
	case reqErr.Parameter != nil:
		fields.Add(reqErr.Parameter.In+"."+reqErr.Parameter.Name, firstReason(reqErr.Err, reqErr.Reason))
	case reqErr.RequestBody != nil:
		if strings.Contains(reqErr.Reason, "Content-Type") {
			*unsupported = true
			return
		}
		collectSchemaErrors(reqErr.Err, nil, fields, reqErr.Reason)
	default:
		fields.Add("request", reqErr.Error())
	}
}

// collectSchemaErrors adds one field error per schema violation, naming the
// field by its JSON path (e.g. workAuthorization.basis).
func collectSchemaErrors(err error, prefix []string, fields *apperr.FieldErrors, fallback string) {
	if multi, ok := err.(openapi3.MultiError); ok { //nolint:errorlint // MultiError is a slice type, not a wrapper
		for _, e := range multi {
			collectSchemaErrors(e, prefix, fields, fallback)
		}
		return
	}
	var schemaErr *openapi3.SchemaError
	if !errors.As(err, &schemaErr) {
		fields.Add(fieldPath(prefix), firstReason(err, fallback))
		return
	}
	path := append(slices.Clone(prefix), schemaErr.JSONPointer()...)
	switch schemaErr.SchemaField {
	case "allOf", "anyOf", "oneOf":
		// A wrapper (e.g. a nullable reference): the real problem is inside,
		// and its path is already complete.
		if schemaErr.Origin != nil {
			collectSchemaErrors(schemaErr.Origin, prefix, fields, schemaErr.Reason)
			return
		}
	}
	// `property "fristName" is unsupported`: name the unexpected field itself.
	if m := unsupportedProperty.FindStringSubmatch(schemaErr.Reason); m != nil {
		path = append(path, m[1])
	}
	fields.Add(fieldPath(path), schemaErr.Reason)
}

var unsupportedProperty = regexp.MustCompile(`^property "([^"]+)" is unsupported$`)

func fieldPath(path []string) string {
	if len(path) == 0 {
		return "body"
	}
	return strings.Join(path, ".")
}

func firstReason(err error, fallback string) string {
	var schemaErr *openapi3.SchemaError
	if errors.As(err, &schemaErr) {
		return schemaErr.Reason
	}
	if fallback != "" {
		return fallback
	}
	if err != nil {
		return err.Error()
	}
	return "invalid"
}
