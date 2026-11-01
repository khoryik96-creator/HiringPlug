// Package authz decides what a signed-in user may do.
//
// Roles and the permissions each role has are declared in the API contract
// (x-roles and x-permissions), next to the endpoints they protect. This
// package reads them from the embedded contract, so there is no second list
// to keep in sync.
//
// Permissions are enforced in three places:
//  1. HTTP: every endpoint's x-permission is checked before the handler runs.
//  2. Services: field-level rules, e.g. salary is only loaded for users with
//     candidates.compensation:read.
//  3. Database: the transaction carries the permissions, and row-level
//     security refuses salary rows to anyone without them.
package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// SystemUserID is the actor recorded for changes made by background workers.
const SystemUserID = "system"

// Principal is who is acting, in which tenant, with which permissions.
type Principal struct {
	TenantID string
	UserID   string
	Email    string
	Name     string
	Role     api.Role
	perms    map[api.Permission]bool
}

// NewPrincipal builds a user principal with the permissions of their role.
func NewPrincipal(tenantID, userID, email, name string, role api.Role) Principal {
	p := Principal{TenantID: tenantID, UserID: userID, Email: email, Name: name, Role: role, perms: map[api.Permission]bool{}}
	for _, perm := range PermissionsForRole(role) {
		p.perms[perm] = true
	}
	return p
}

// System is the principal background workers use for a tenant. It has every
// permission; what a job may change is decided by the job's own code.
func System(tenantID string) Principal {
	p := Principal{TenantID: tenantID, UserID: SystemUserID, Name: "System", perms: map[api.Permission]bool{}}
	for perm := range catalog().permissions {
		p.perms[perm] = true
	}
	return p
}

// TenantOnly is a principal with no permissions, used while looking up the
// user behind a token.
func TenantOnly(tenantID string) Principal {
	return Principal{TenantID: tenantID, UserID: SystemUserID, perms: map[api.Permission]bool{}}
}

// Can reports whether the principal has perm.
func (p Principal) Can(perm api.Permission) bool { return p.perms[perm] }

// Permissions returns the principal's permissions, sorted.
func (p Principal) Permissions() []api.Permission {
	out := make([]api.Permission, 0, len(p.perms))
	for perm := range p.perms {
		out = append(out, perm)
	}
	slices.Sort(out)
	return out
}

// PermissionCSV is the form stored in the transaction setting app.permissions.
func (p Principal) PermissionCSV() string {
	perms := p.Permissions()
	s := make([]string, len(perms))
	for i, perm := range perms {
		s[i] = string(perm)
	}
	return strings.Join(s, ",")
}

// PermissionsForRole returns the permissions the contract grants role.
func PermissionsForRole(role api.Role) []api.Permission {
	return slices.Clone(catalog().byRole[role])
}

// IsSensitive reports whether perm is marked sensitive in the contract.
func IsSensitive(perm api.Permission) bool { return catalog().permissions[perm].Sensitive }

type permissionInfo struct {
	Module    string   `json:"module"`
	Sensitive bool     `json:"sensitive"`
	Roles     []string `json:"roles"`
}

type permissionCatalog struct {
	permissions map[api.Permission]permissionInfo
	byRole      map[api.Role][]api.Permission
}

var catalog = sync.OnceValue(func() permissionCatalog {
	spec, err := api.Spec()
	if err != nil {
		panic(fmt.Sprintf("authz: embedded API contract does not load: %v", err))
	}
	raw, err := json.Marshal(spec.Extensions["x-permissions"])
	if err != nil {
		panic(fmt.Sprintf("authz: x-permissions: %v", err))
	}
	var perms map[string]permissionInfo
	if err := json.Unmarshal(raw, &perms); err != nil {
		panic(fmt.Sprintf("authz: x-permissions: %v", err))
	}
	c := permissionCatalog{permissions: map[api.Permission]permissionInfo{}, byRole: map[api.Role][]api.Permission{}}
	for name, info := range perms {
		c.permissions[api.Permission(name)] = info
		for _, role := range info.Roles {
			c.byRole[api.Role(role)] = append(c.byRole[api.Role(role)], api.Permission(name))
		}
	}
	for role := range c.byRole {
		sort.Slice(c.byRole[role], func(i, j int) bool { return c.byRole[role][i] < c.byRole[role][j] })
	}
	return c
})

type ctxKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the principal set by the auth middleware.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// MustFromContext is for handlers of endpoints that require sign-in: the
// middleware guarantees a principal, so a missing one is a programming error.
func MustFromContext(ctx context.Context) Principal {
	p, ok := FromContext(ctx)
	if !ok {
		panic("authz: no principal in context; is the endpoint marked x-permission: public?")
	}
	return p
}
