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
	err := admin.QueryRow(ctx, `INSERT INTO tenant (name) VALUES ($1) RETURNING id`, strings.TrimSpace(name)).Scan(&id)
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
