package app

// Administrative operations for the `hp` command and tests. They use the
// admin (owner) connection, which is not subject to row-level security, so
// they must never be reachable from the HTTP API.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
)

// Group roles created by migration 00001.
const (
	GroupApp = "hiringplug_app"
	GroupAI  = "hiringplug_ai"
)

// EnsureLoginRole creates (or updates the password of) the login role in
// loginURL and makes it a member of group. For development and tests only:
// production creates its login roles with its own secrets.
func EnsureLoginRole(ctx context.Context, admin *pgxpool.Pool, loginURL, group string) error {
	u, err := url.Parse(loginURL)
	if err != nil || u.User == nil {
		return fmt.Errorf("login URL must include a user and password")
	}
	user := u.User.Username()
	password, _ := u.User.Password()
	if user == "" || password == "" {
		return fmt.Errorf("login URL must include a user and password")
	}
	// Several processes (test packages run in parallel, each with its own
	// database in one PostgreSQL) may set up the same role at once:
	// PostgreSQL then refuses one ("tuple concurrently updated", or the role
	// made meanwhile), so it tries again.
	for attempt := range 20 {
		if err = setLoginRole(ctx, admin, user, password, group); !retryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 20 * time.Millisecond):
		}
	}
	return err
}

func setLoginRole(ctx context.Context, admin *pgxpool.Pool, user, password, group string) error {
	// format() quotes the identifier and literal safely on the server side.
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, user).Scan(&exists); err != nil {
		return err
	}
	stmt := `SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', $1::text, $2::text)`
	if !exists {
		stmt = `SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', $1::text, $2::text)`
	}
	if err := execFormatted(ctx, admin, stmt, user, password); err != nil {
		return fmt.Errorf("creating login role %s: %w", user, err)
	}
	if err := execFormatted(ctx, admin, `SELECT format('GRANT %I TO %I', $1::text, $2::text)`, group, user); err != nil {
		return fmt.Errorf("granting %s to %s: %w", group, user, err)
	}
	return nil
}

// retryable: another process changed the same role at the same moment.
func retryable(err error) bool {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return false
	}
	return pg.Code == "42710" || // made meanwhile (duplicate_object)
		(pg.Code == "23505" && pg.ConstraintName == "pg_authid_rolname_index") || // the same, caught later
		(pg.Code == "XX000" && strings.Contains(pg.Message, "concurrently updated"))
}

func execFormatted(ctx context.Context, admin *pgxpool.Pool, formatSQL string, args ...any) error {
	var stmt string
	if err := admin.QueryRow(ctx, formatSQL, args...).Scan(&stmt); err != nil {
		return err
	}
	_, err := admin.Exec(ctx, stmt)
	return err
}

// CreateTenant adds a tenant and returns its ID.
func CreateTenant(ctx context.Context, admin *pgxpool.Pool, name string) (string, error) {
	var id string
	var err error
	for range 10 {
		err = admin.QueryRow(ctx, `INSERT INTO tenant (name) VALUES ($1) RETURNING id`, strings.TrimSpace(name)).Scan(&id)
		// Its careers address comes from its name ("-2", "-3" when taken): two
		// made at the same moment with the same name may pick the same one. The
		// next try takes the next free one.
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.ConstraintName != "tenant_careers_slug_key" {
			return id, err
		}
	}
	return id, err
}

// CreateUser adds a user to a tenant and returns their ID.
func CreateUser(ctx context.Context, admin *pgxpool.Pool, tenantID, email, name string, role api.Role) (string, error) {
	if !role.Valid() {
		return "", fmt.Errorf("unknown role %q", role)
	}
	var id string
	err := admin.QueryRow(ctx,
		`INSERT INTO app_user (tenant_id, email, name, role) VALUES ($1, lower($2), $3, $4) RETURNING id`,
		tenantID, strings.TrimSpace(email), strings.TrimSpace(name), string(role)).Scan(&id)
	return id, err
}

// UserTenant returns the tenant a user belongs to.
func UserTenant(ctx context.Context, admin *pgxpool.Pool, userID string) (string, error) {
	var tenantID string
	err := admin.QueryRow(ctx, `SELECT tenant_id FROM app_user WHERE id = $1`, userID).Scan(&tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("no user with id %s", userID)
	}
	return tenantID, err
}

// PasswordLink makes a link for a user to choose their password (an
// invitation if they have none yet, else a reset), valid for valid: for the
// first owner, or anyone the app cannot email.
func PasswordLink(ctx context.Context, admin *pgxpool.Pool, cfg config.Config, userID string, valid time.Duration) (string, error) {
	tenantID, err := UserTenant(ctx, admin, userID)
	if err != nil {
		return "", err
	}
	m := identity.New(admin, authn.NewVerifier(cfg.AuthJWTSecret, cfg.AuthIssuer, cfg.AuthAudience), nil, cfg.PublicURL)
	return m.PasswordLink(ctx, tenantID, userID, valid)
}

// SetPassword sets a user's password directly (development demo users only).
func SetPassword(ctx context.Context, admin *pgxpool.Pool, userID, password string) error {
	hash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = admin.Exec(ctx, `UPDATE app_user SET password_hash = $2, invited = false WHERE id = $1`, userID, hash)
	return err
}

// ExplainAdminLogin turns "password authentication failed" for the admin
// connection into what to do: with Docker, the admin password is fixed when
// the database is first created, so setting HP_DB_ADMIN_PASSWORD later does
// not change it. Other errors are returned as they are.
func ExplainAdminLogin(err error) error {
	var pgErr *pgconn.PgError
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != "28P01" {
		return err
	}
	return fmt.Errorf("the database refused the admin password (HP_DATABASE_ADMIN_URL). "+
		"With Docker, the admin password is set only when the database is first created: "+
		"if you set or changed HP_DB_ADMIN_PASSWORD after that, either put back the password it was created with "+
		"(\"postgres\" for a laptop setup), or change it inside the database first "+
		"(docker compose -f deploy/docker-compose.yml exec postgres psql -U postgres -c \"ALTER USER postgres PASSWORD '<new>'\"): %w", err)
}
