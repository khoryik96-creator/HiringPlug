// Command hp is the HiringPlug admin tool.
//
//	hp migrate                         apply database migrations (uses HP_DATABASE_ADMIN_URL)
//	hp bootstrap                       migrate, then create/update the login roles in HP_DATABASE_URL and HP_AI_DATABASE_URL
//	hp dev-setup                       local development: bootstrap, plus a demo tenant with users, and print their tokens
//	hp tenant -name "Acme Recruitment" -owner-email you@acme.com -owner-name "Your Name"
//	hp user -tenant tnt_... -email a@b.com -name "A B" -role recruiter
//	hp password-link -user usr_...   (a link to choose a password)
//	hp token -user usr_... [-ttl 12h]  print a sign-in token for a user (development)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/db"
	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "hp:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: hp <migrate|bootstrap|dev-setup|tenant|user|token|password-link> [flags]")
}

func run(ctx context.Context, cmd string, args []string) error {
	switch cmd {
	case "migrate":
		return database.Migrate(ctx, adminURL(), db.Migrations())
	case "bootstrap":
		return bootstrap(ctx)
	case "dev-setup":
		return devSetup(ctx)
	case "tenant":
		return createTenant(ctx, args)
	case "user":
		return createUser(ctx, args)
	case "token":
		return printToken(ctx, args)
	case "password-link":
		return printPasswordLink(ctx, args)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func adminURL() string { return os.Getenv("HP_DATABASE_ADMIN_URL") }

func adminPool(ctx context.Context) (*pgxpool.Pool, error) {
	if adminURL() == "" {
		return nil, errors.New("HP_DATABASE_ADMIN_URL is required (the database owner/admin connection)")
	}
	return database.Connect(ctx, adminURL())
}

// bootstrap prepares a database for the app: migrations, then the login roles
// for the API/worker (HP_DATABASE_URL) and the AI worker (HP_AI_DATABASE_URL).
// Safe to run on every deploy.
func bootstrap(ctx context.Context) error {
	if err := database.Migrate(ctx, adminURL(), db.Migrations()); err != nil {
		return err
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	appURL := os.Getenv("HP_DATABASE_URL")
	if appURL == "" {
		return errors.New("HP_DATABASE_URL is required")
	}
	if err := app.EnsureLoginRole(ctx, admin, appURL, app.GroupApp); err != nil {
		return err
	}
	if aiURL := os.Getenv("HP_AI_DATABASE_URL"); aiURL != "" {
		if err := app.EnsureLoginRole(ctx, admin, aiURL, app.GroupAI); err != nil {
			return err
		}
	}
	fmt.Println("database ready")
	return nil
}

// devSetup prepares a local database: bootstrap, then a demo tenant with one
// user per role, and prints sign-in tokens for them.
func devSetup(ctx context.Context) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if cfg.Env == config.EnvProduction {
		return errors.New("dev-setup refuses to run with HP_ENV=production")
	}
	if err := bootstrap(ctx); err != nil {
		return err
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()

	var tenantID string
	err = admin.QueryRow(ctx, `SELECT id FROM tenant WHERE name = 'Demo Recruitment' ORDER BY id LIMIT 1`).Scan(&tenantID)
	if database.IsNoRows(err) {
		tenantID, err = app.CreateTenant(ctx, admin, "Demo Recruitment")
	}
	if err != nil {
		return err
	}
	verifier := authn.NewVerifier(cfg.AuthJWTSecret, cfg.AuthIssuer, cfg.AuthAudience)
	fmt.Printf("Demo tenant %s\n\nSign in with these emails and the password %q, or the tokens (valid 30 days, development only):\n", tenantID, devPassword)
	for _, role := range []api.Role{api.RoleOwner, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer} {
		email := string(role) + "@demo.hiringplug.local"
		var userID string
		err := admin.QueryRow(ctx, `SELECT id FROM app_user WHERE tenant_id = $1 AND email = $2`, tenantID, email).Scan(&userID)
		if database.IsNoRows(err) {
			userID, err = app.CreateUser(ctx, admin, tenantID, email, "Demo "+string(role), role)
		}
		if err != nil {
			return err
		}
		if err := app.SetPassword(ctx, admin, userID, devPassword); err != nil {
			return err
		}
		token, err := verifier.Mint(tenantID, userID, 30*24*time.Hour)
		if err != nil {
			return err
		}
		fmt.Printf("\n%-10s %s\n%s\n", role, email, token)
	}
	return nil
}

// devPassword is the demo users' password (development databases only).
const devPassword = "demo-password-2026"

func createTenant(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tenant", flag.ContinueOnError)
	name := fs.String("name", "", "tenant (company) name")
	ownerEmail := fs.String("owner-email", "", "first user's email")
	ownerName := fs.String("owner-name", "", "first user's name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *ownerEmail == "" || *ownerName == "" {
		return errors.New("-name, -owner-email and -owner-name are required")
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	tenantID, err := app.CreateTenant(ctx, admin, *name)
	if err != nil {
		return err
	}
	userID, err := app.CreateUser(ctx, admin, tenantID, *ownerEmail, *ownerName, api.RoleOwner)
	if err != nil {
		return err
	}
	fmt.Printf("tenant %s\nowner  %s\n", tenantID, userID)
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	link, err := app.PasswordLink(ctx, admin, cfg, userID, 7*24*time.Hour)
	if err != nil {
		return err
	}
	fmt.Printf("\nThe owner chooses their password here (valid 7 days):\n%s\n", link)
	return nil
}

func createUser(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user", flag.ContinueOnError)
	tenant := fs.String("tenant", "", "tenant ID (tnt_...)")
	email := fs.String("email", "", "email")
	name := fs.String("name", "", "name")
	role := fs.String("role", "recruiter", "owner | admin | recruiter | sourcer | hiring_manager | viewer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	userID, err := app.CreateUser(ctx, admin, *tenant, *email, *name, api.Role(*role))
	if err != nil {
		return err
	}
	fmt.Println(userID)
	return nil
}

func printToken(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	user := fs.String("user", "", "user ID (usr_...)")
	ttl := fs.Duration("ttl", 12*time.Hour, "how long the token is valid")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	tenantID, err := app.UserTenant(ctx, admin, *user)
	if err != nil {
		return err
	}
	token, err := authn.NewVerifier(cfg.AuthJWTSecret, cfg.AuthIssuer, cfg.AuthAudience).Mint(tenantID, *user, *ttl)
	if err != nil {
		return err
	}
	fmt.Println(token)
	return nil
}

// printPasswordLink prints a link for a user to choose their password: the
// first owner, someone the app cannot email, or a forgotten password when
// email is not set up.
func printPasswordLink(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("password-link", flag.ContinueOnError)
	user := fs.String("user", "", "user ID (usr_...)")
	valid := fs.Duration("valid", 24*time.Hour, "how long the link works")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	link, err := app.PasswordLink(ctx, admin, cfg, *user, *valid)
	if err != nil {
		return err
	}
	fmt.Println(link)
	return nil
}
