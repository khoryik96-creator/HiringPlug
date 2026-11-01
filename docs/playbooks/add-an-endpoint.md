# Add an endpoint or a field

The contract comes first; the compiler then guides you.

## A new endpoint

1. Add the path to the module's file in `contracts/modules/`, with an
   `operationId` and an `x-permission` (an existing permission of the module,
   or a new one declared under `x-module.permissions` with its default roles).
2. `task gen`. Go now fails to compile:
   `*Server does not implement api.StrictServerInterface (missing method ...)`.
   That method is your handler.
3. Write it in the module's `http.go`: convert the request to domain values, call
   the service, convert the result. Put rules in `domain.go`, data access in
   `db/queries/<module>.sql` (then `task gen`), and the use case in
   `service.go`.
4. Test it in `internal/app/<module>_test.go` through the HTTP API. Responses are
   automatically checked against the contract.

## A new field on an existing record

1. Add it to the schemas in the module contract (response, create, update).
2. Add a migration (a new file) and update the queries; `task gen`.
3. Update `domain.go` (rules), `service.go` (mapping), `http.go` (mapping).
   Compile errors show every place that needs it, including the web app and the
   Python models.
4. Sensitive field? Follow [access-control.md](../access-control.md#adding-a-sensitive-field).

## A new background job

Declare it under `x-module.jobs` (queue `core` for Go, `ai` for Python) with a
payload schema, run `task gen`, queue it with `jobqueue.Enqueue` inside your
service's transaction, and add the handler: Go in the module's `JobHandlers()`,
Python in `ai/src/hiringplug_ai/jobs/`. Tests fail until a handler exists.
