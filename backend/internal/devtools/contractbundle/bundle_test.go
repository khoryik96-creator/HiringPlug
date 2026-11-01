package contractbundle

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRepoContractIsUpToDate fails when a module contract was edited without
// running `task gen`, so every generator always reads the real contract.
func TestRepoContractIsUpToDate(t *testing.T) {
	bundled, err := BundleFiles("../../../../contracts/openapi.base.yaml", "../../../../contracts/modules")
	require.NoError(t, err)
	committed, err := os.ReadFile("../../../../contracts/openapi.yaml")
	require.NoError(t, err)
	assert.Equal(t, string(committed), string(bundled), ErrOutOfDate.Error())
}

const base = `
openapi: 3.0.3
info: { title: t, version: "1" }
x-roles: { owner: Owner, viewer: Viewer }
x-error-codes:
  NOT_FOUND: { status: 404, description: Not found. }
paths: {}
components:
  schemas:
    Role: { type: string, enum: [x] }
    Permission: { type: string, enum: [x] }
    ErrorCode: { type: string, enum: [x] }
    JobKind: { type: string, enum: [x] }
`

func module(name, permissions, paths, schemas string) string {
	return `
x-module:
  name: ` + name + `
  description: The ` + name + ` module.
  permissions: {` + permissions + `}
  errorCodes: {}
  jobs: {}
paths: {` + paths + `}
components:
  schemas: {` + schemas + `}
`
}

func bundle(t *testing.T, modules map[string]string) error {
	t.Helper()
	parse := func(s string) *yaml.Node {
		var n yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(s), &n))
		return &n
	}
	var docs []namedDoc
	for _, name := range []string{"notes", "tasks"} {
		if src, ok := modules[name]; ok {
			docs = append(docs, namedDoc{name: name + ".yaml", doc: parse(src)})
		}
	}
	_, err := Bundle(namedDoc{name: "base.yaml", doc: parse(base)}, docs)
	return err
}

const notesPerm = `"notes:read": { description: Read notes., roles: [owner, viewer] }`
const notesPath = `"/v1/notes": { get: { operationId: listNotes, x-permission: "notes:read", responses: { "200": { description: OK } } } }`

func TestAValidModuleBundles(t *testing.T) {
	assert.NoError(t, bundle(t, map[string]string{"notes": module("notes", notesPerm, notesPath, `Note: { type: object }`)}))
}

func TestModulesCannotCollide(t *testing.T) {
	err := bundle(t, map[string]string{
		"notes": module("notes", notesPerm, notesPath, `Note: { type: object }`),
		"tasks": module("tasks", `"tasks:read": { description: Read., roles: [owner] }`,
			`"/v1/notes": { get: { operationId: listTasks, x-permission: "tasks:read", responses: { "200": { description: OK } } } }`,
			`Note: { type: object }`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `path "/v1/notes" is already defined by module "notes"`)
	assert.Contains(t, err.Error(), `components.schemas.Note is already defined by module "notes"`)
}

func TestPermissionsBelongToTheirModule(t *testing.T) {
	err := bundle(t, map[string]string{"notes": module("notes", `"candidates:read": { description: Sneaky., roles: [owner] }`, "", "")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `must start with the module name "notes"`)
}

func TestEveryOperationNeedsAKnownPermission(t *testing.T) {
	missing := `"/v1/notes": { get: { operationId: listNotes, responses: { "200": { description: OK } } } }`
	unknown := `"/v1/notes": { get: { operationId: listNotes, x-permission: "notes:write", responses: { "200": { description: OK } } } }`
	err := bundle(t, map[string]string{"notes": module("notes", notesPerm, missing, "")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "x-permission is required")
	err = bundle(t, map[string]string{"notes": module("notes", notesPerm, unknown, "")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `uses x-permission "notes:write", which no module declares`)
}

func TestRolesMustExist(t *testing.T) {
	err := bundle(t, map[string]string{"notes": module("notes", `"notes:read": { description: Read., roles: [superhero] }`, "", "")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown role "superhero"`)
}

func TestAllProblemsAreReportedTogether(t *testing.T) {
	err := bundle(t, map[string]string{"notes": module("tasks", `"bad name": { description: x, roles: [owner] }`, "", "")})
	require.Error(t, err)
	assert.Equal(t, 2, strings.Count(err.Error(), "\n  - "), err.Error())
}
