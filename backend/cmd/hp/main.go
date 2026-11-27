// Command hp is the HyPlug admin tool.
//
//	hp migrate                         apply database migrations (uses HP_DATABASE_ADMIN_URL)
//	hp bootstrap                       migrate, then create/update the login roles in HP_DATABASE_URL and HP_AI_DATABASE_URL
//	hp dev-setup                       local development: bootstrap, plus a demo tenant with users, and print their tokens
//	hp tenant -name "Acme Recruitment" -owner-email you@acme.com -owner-name "Your Name"
//	hp user -tenant tnt_... -email a@b.com -name "A B" -role recruiter
//	hp password-link -user usr_...   (a link to choose a password)
//	hp token -user usr_... [-ttl 12h]  print a sign-in token for a user (development)
//	hp backup [-every 24h]             back up the database and files into HP_BACKUP_DIR
//	hp restore -yes NAME               put a backup back (stop the API and workers first); -resume-outbound, -resume-paid-ai let held jobs run
//	hp fetch [NAME]                    copy a backup from off the server into HP_BACKUP_DIR (the newest unless named); then hp restore it
//	hp repair-files [-apply]           check (and fix) that the API's user owns the uploaded files (HP_FILES_OWNER; stop the API and workers first)
//	hp jobs list|release|cancel        the jobs a restore held, every company's: list them, let them run (-yes), or cancel them
//	hp paid-ai on|off [-reason TEXT]   turn the server's paid AI back on, or pause it (every company)
//
// Flags go before or after the names (hp restore NAME -yes works too); -h
// prints a command's usage.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
		usage(os.Stderr)
		os.Exit(2)
	}
	os.Exit(exitCode(run(ctx, os.Args[1], os.Args[2:]), os.Stderr))
}

// exitCode is 0 for success and for -h, 1 for any failure (said on w), so a
// script stops at a failed restore.
func exitCode(err error, w io.Writer) int {
	switch {
	case err == nil, isHelp(err):
		return 0
	default:
		_, _ = fmt.Fprintln(w, "hp:", err)
		return 1
	}
}

const commands = "migrate|bootstrap|dev-setup|tenant|user|token|password-link|backup|restore|fetch|repair-files|jobs|paid-ai"

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: hp <"+commands+"> [flags] (hp <command> -h for its flags)")
}

// noArgs is a command that takes neither flags nor names.
func noArgs(name string, args []string) error {
	c := command{name: name, synopsis: ""}
	_, err := c.parse(c.flags(), args)
	return err
}

func run(ctx context.Context, cmd string, args []string) error {
	switch cmd {
	case "migrate":
		if err := noArgs(cmd, args); err != nil {
			return err
		}
		return database.Migrate(ctx, adminURL(), db.Migrations())
	case "bootstrap":
		if err := noArgs(cmd, args); err != nil {
			return err
		}
		return bootstrap(ctx)
	case "dev-setup":
		if err := noArgs(cmd, args); err != nil {
			return err
		}
		return devSetup(ctx)
	case "tenant":
		return createTenant(ctx, args)
	case "user":
		return createUser(ctx, args)
	case "token":
		return printToken(ctx, args)
	case "password-link":
		return printPasswordLink(ctx, args)
	case "backup":
		return runBackup(ctx, args)
	case "fetch":
		return runFetch(ctx, args)
	case "restore":
		return runRestore(ctx, args)
	case "repair-files":
		return runRepairFiles(ctx, args)
	case "jobs":
		return runJobs(ctx, args)
	case "paid-ai":
		return runPaidAI(ctx, args)
	case "-h", "-help", "--help", "help":
		usage(os.Stdout)
		return flag.ErrHelp
	default:
		usage(os.Stderr)
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
		return app.ExplainAdminLogin(err)
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return app.ExplainAdminLogin(err)
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
	c := command{name: "tenant", synopsis: "-name NAME -owner-email EMAIL -owner-name NAME", min: 0, max: 0}
	fs := c.flags()
	name := fs.String("name", "", "tenant (company) name")
	ownerEmail := fs.String("owner-email", "", "first user's email")
	ownerName := fs.String("owner-name", "", "first user's name")
	if _, err := c.parse(fs, args); err != nil {
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
	c := command{name: "user", synopsis: "-tenant tnt_... -email EMAIL -name NAME [-role recruiter]", min: 0, max: 0}
	fs := c.flags()
	tenant := fs.String("tenant", "", "tenant ID (tnt_...)")
	email := fs.String("email", "", "email")
	name := fs.String("name", "", "name")
	role := fs.String("role", "recruiter", "owner | admin | recruiter | sourcer | hiring_manager | viewer")
	if _, err := c.parse(fs, args); err != nil {
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
	c := command{name: "token", synopsis: "-user usr_... [-ttl 12h]", min: 0, max: 0}
	fs := c.flags()
	user := fs.String("user", "", "user ID (usr_...)")
	ttl := fs.Duration("ttl", 12*time.Hour, "how long the token is valid")
	if _, err := c.parse(fs, args); err != nil {
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
	c := command{name: "password-link", synopsis: "-user usr_... [-valid 24h]", min: 0, max: 0}
	fs := c.flags()
	user := fs.String("user", "", "user ID (usr_...)")
	valid := fs.Duration("valid", 24*time.Hour, "how long the link works")
	if _, err := c.parse(fs, args); err != nil {
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
