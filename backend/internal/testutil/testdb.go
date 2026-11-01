// Package testutil gives integration tests a real, freshly migrated
// PostgreSQL database. Tests never mock the database: the bugs that matter
// (constraints, row-level security, transactions) only show up in a real one.
//
// Each test package gets its own database (created in TestMain, dropped
// afterwards), and each test should create its own tenant, so tests can run
// in parallel without seeing each other's data.
//
// Set HP_TEST_ADMIN_URL to a PostgreSQL superuser URL; the default matches
// `task db:up`. If the database is unreachable the tests fail (they never
// silently skip) unless HP_SKIP_DB_TESTS=1.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/db"
	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const (
	defaultAdminURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" //nolint:gosec // local test database only
	// Test-only login roles (cluster-wide, shared by all test databases).
	appLogin = "hp_api_test"
	aiLogin  = "hp_ai_test"
	// JWTSecret signs test tokens.
	JWTSecret = "test-only-secret-0123456789abcdefghijklmnop"
)

// DB is a migrated test database.
type DB struct {
	Name     string
	AdminURL string // owner connection (bypasses RLS) for setup
	AppURL   string // restricted app role, exactly like production
	AIURL    string // Python worker role: may only call job functions
	Admin    *pgxpool.Pool
	App      *pgxpool.Pool
}

// Current is the package's database, set by Main.
var Current *DB

// Main creates the package database, runs the tests, then drops it. Use it
// from TestMain: func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }
func Main(m *testing.M) int {
	if os.Getenv("HP_SKIP_DB_TESTS") == "1" {
		fmt.Println("HP_SKIP_DB_TESTS=1: skipping database tests")
		return 0
	}
	ctx := context.Background()
	d, cleanup, err := create(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nintegration tests need PostgreSQL: %v\n"+
			"Start it with `task db:up`, or point HP_TEST_ADMIN_URL at a superuser connection.\n\n", err)
		return 1
	}
	Current = d
	code := m.Run()
	cleanup()
	return code
}

func create(ctx context.Context) (*DB, func(), error) {
	rootURL := os.Getenv("HP_TEST_ADMIN_URL")
	if rootURL == "" {
		rootURL = defaultAdminURL
	}
	root, err := database.Connect(ctx, rootURL)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "hp_test_" + hex.EncodeToString(suffix)
	if _, err := root.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return nil, nil, err
	}
	d := &DB{Name: name, AdminURL: withDatabase(rootURL, name, "", "")}
	d.AppURL = withDatabase(rootURL, name, appLogin, appLogin)
	d.AIURL = withDatabase(rootURL, name, aiLogin, aiLogin)

	drop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if d.App != nil {
			d.App.Close()
		}
		if d.Admin != nil {
			d.Admin.Close()
		}
		if r, err := database.Connect(ctx, rootURL); err == nil {
			_, _ = r.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			r.Close()
		}
	}
	fail := func(err error) (*DB, func(), error) { drop(); return nil, nil, err }

	if err := database.Migrate(ctx, d.AdminURL, db.Migrations()); err != nil {
		return fail(err)
	}
	if d.Admin, err = database.Connect(ctx, d.AdminURL); err != nil {
		return fail(err)
	}
	// Login roles are cluster-wide; serialize their creation across packages.
	if _, err := d.Admin.Exec(ctx, "SELECT pg_advisory_lock(424242)"); err != nil {
		return fail(err)
	}
	roleErr := app.EnsureLoginRole(ctx, d.Admin, d.AppURL, app.GroupApp)
	if roleErr == nil {
		roleErr = app.EnsureLoginRole(ctx, d.Admin, d.AIURL, app.GroupAI)
	}
	_, _ = d.Admin.Exec(ctx, "SELECT pg_advisory_unlock(424242)")
	if roleErr != nil {
		return fail(roleErr)
	}
	if d.App, err = database.Connect(ctx, d.AppURL); err != nil {
		return fail(err)
	}
	return d, drop, nil
}

func withDatabase(raw, dbName, user, password string) string {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + dbName
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
	return u.String()
}

// Tenant is a test tenant with one user per requested role.
type Tenant struct {
	ID    string
	Users map[api.Role]User
}

// User is a test user with a valid sign-in token.
type User struct {
	ID    string
	Role  api.Role
	Token string
}

// Verifier signs tokens the test app accepts.
func Verifier() *authn.Verifier {
	return authn.NewVerifier([]byte(JWTSecret), "hiringplug", "hiringplug-api")
}

// NewTenant creates a tenant with a user for each role.
func (d *DB) NewTenant(t testing.TB, roles ...api.Role) Tenant {
	t.Helper()
	ctx := context.Background()
	tenantID, err := app.CreateTenant(ctx, d.Admin, "Test tenant "+t.Name())
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}
	tenant := Tenant{ID: tenantID, Users: map[api.Role]User{}}
	for _, role := range roles {
		email := fmt.Sprintf("%s.%s@test.local", role, tenantID)
		userID, err := app.CreateUser(ctx, d.Admin, tenantID, email, "Test "+string(role), role)
		if err != nil {
			t.Fatalf("creating user: %v", err)
		}
		token, err := Verifier().Mint(tenantID, userID, time.Hour)
		if err != nil {
			t.Fatalf("minting token: %v", err)
		}
		tenant.Users[role] = User{ID: userID, Role: role, Token: token}
	}
	return tenant
}
