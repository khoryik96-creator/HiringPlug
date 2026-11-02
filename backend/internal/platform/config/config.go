// Package config reads all settings from environment variables, once, at
// startup. Every variable is listed here and in .env.example; if any is
// missing or invalid the program refuses to start and says exactly which.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	// Time zone names work wherever the app runs, even without the
	// system's time zone files (Windows, minimal images).
	_ "time/tzdata"
)

// Environments.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// Config holds every setting the API and the worker need.
type Config struct {
	Env string // HP_ENV: development | test | production

	HTTPAddr        string        // HP_HTTP_ADDR, e.g. :8080
	ShutdownTimeout time.Duration // HP_SHUTDOWN_TIMEOUT, e.g. 15s

	// DatabaseURL connects as the restricted application role (row-level
	// security applies). Migrations use a separate admin URL (see cmd/hp).
	DatabaseURL string // HP_DATABASE_URL

	LogLevel  slog.Level // HP_LOG_LEVEL: debug | info | warn | error
	LogFormat string     // HP_LOG_FORMAT: json | text

	AuthJWTSecret []byte // HP_AUTH_JWT_SECRET, at least 32 characters
	AuthIssuer    string // HP_AUTH_ISSUER
	AuthAudience  string // HP_AUTH_AUDIENCE

	FilesDir       string // HP_FILES_DIR: where uploaded files are stored
	FilesURLSecret []byte // HP_FILES_URL_SECRET: signs download links for workers
	// InternalBaseURL is how workers reach this API to download files,
	// e.g. http://localhost:8080 or http://api:8080 inside Docker.
	InternalBaseURL  string // HP_INTERNAL_BASE_URL
	MaxUploadBytes   int64  // HP_MAX_UPLOAD_BYTES
	DefaultPhoneArea string // HP_DEFAULT_PHONE_REGION: ISO country used for local phone numbers, e.g. MY
	// TimeZone is where the company works: times in emails the app sends by
	// itself (interview reminders) are written in it.
	TimeZone *time.Location // HP_TIME_ZONE, e.g. Asia/Kuala_Lumpur
	// ClientIPHeader is the header a reverse proxy names the client in (nginx:
	// X-Real-IP). Empty when the API is reached directly. Never a header
	// clients could set themselves.
	ClientIPHeader string // HP_CLIENT_IP_HEADER
	// PublicURL is where people reach the web app, e.g. https://hire.example.com:
	// links in job feeds (and later emails) point there.
	PublicURL string // HP_PUBLIC_URL
	// SMTP is the mail server email from the app is sent through. Without a
	// host, emails are recorded but fail with "no mail server is set up".
	SMTP SMTP
}

// SMTP is how to reach the mail server.
type SMTP struct {
	Addr     string // HP_SMTP_HOST: host:port, e.g. smtp.example.com:587
	Username string // HP_SMTP_USERNAME
	Password string // HP_SMTP_PASSWORD
	From     string // HP_SMTP_FROM: the address mail is sent from, e.g. no-reply@example.com
	TLS      string // HP_SMTP_TLS: starttls (default), tls (port 465) or none (local mail catchers only)
}

// devSecret is only accepted outside production.
const devSecret = "dev-only-secret-change-me-0123456789abcdef" //nolint:gosec // placeholder; production refuses it

// Load reads the configuration using getenv (pass os.Getenv). It reports every
// problem at once rather than stopping at the first.
func Load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}
	c := Config{
		Env:              r.oneOf("HP_ENV", EnvDevelopment, EnvDevelopment, EnvTest, EnvProduction),
		HTTPAddr:         r.str("HP_HTTP_ADDR", ":8080"),
		ShutdownTimeout:  r.duration("HP_SHUTDOWN_TIMEOUT", 15*time.Second),
		DatabaseURL:      r.url("HP_DATABASE_URL", ""),
		LogFormat:        r.oneOf("HP_LOG_FORMAT", "json", "json", "text"),
		AuthJWTSecret:    []byte(r.str("HP_AUTH_JWT_SECRET", devSecret)),
		AuthIssuer:       r.str("HP_AUTH_ISSUER", "hiringplug"),
		AuthAudience:     r.str("HP_AUTH_AUDIENCE", "hiringplug-api"),
		FilesDir:         r.str("HP_FILES_DIR", "./data/files"),
		FilesURLSecret:   []byte(r.str("HP_FILES_URL_SECRET", devSecret)),
		InternalBaseURL:  strings.TrimRight(r.url("HP_INTERNAL_BASE_URL", "http://localhost:8080"), "/"),
		MaxUploadBytes:   r.int64("HP_MAX_UPLOAD_BYTES", 10<<20),
		DefaultPhoneArea: strings.ToUpper(r.str("HP_DEFAULT_PHONE_REGION", "MY")),
		ClientIPHeader:   r.str("HP_CLIENT_IP_HEADER", ""),
		PublicURL:        strings.TrimRight(r.url("HP_PUBLIC_URL", "http://localhost:5173"), "/"),
	}
	c.LogLevel = r.level("HP_LOG_LEVEL", slog.LevelInfo)
	zone := r.str("HP_TIME_ZONE", "Asia/Kuala_Lumpur")
	if loc, err := time.LoadLocation(zone); err != nil || zone == "Local" {
		r.fail("HP_TIME_ZONE must be a time zone name, e.g. Asia/Kuala_Lumpur")
	} else {
		c.TimeZone = loc
	}
	c.SMTP = SMTP{
		Addr: r.str("HP_SMTP_HOST", ""), Username: r.str("HP_SMTP_USERNAME", ""), Password: r.str("HP_SMTP_PASSWORD", ""),
		From: r.str("HP_SMTP_FROM", ""), TLS: r.oneOf("HP_SMTP_TLS", "starttls", "starttls", "tls", "none"),
	}
	if c.SMTP.Addr != "" {
		if _, _, err := net.SplitHostPort(c.SMTP.Addr); err != nil {
			r.fail("HP_SMTP_HOST must be host:port, e.g. smtp.example.com:587")
		}
		if a, err := mail.ParseAddress(c.SMTP.From); err != nil || a.Address != c.SMTP.From {
			r.fail("HP_SMTP_FROM must be an email address, e.g. no-reply@example.com")
		}
	}

	if c.DatabaseURL == "" {
		r.fail("HP_DATABASE_URL is required (the restricted application role, not the admin)")
	}
	if len(c.AuthJWTSecret) < 32 {
		r.fail("HP_AUTH_JWT_SECRET must be at least 32 characters")
	}
	if len(c.FilesURLSecret) < 32 {
		r.fail("HP_FILES_URL_SECRET must be at least 32 characters")
	}
	if c.MaxUploadBytes <= 0 {
		r.fail("HP_MAX_UPLOAD_BYTES must be positive")
	}
	if len(c.DefaultPhoneArea) != 2 {
		r.fail("HP_DEFAULT_PHONE_REGION must be a 2-letter country code, e.g. MY")
	}
	if c.Env == EnvProduction {
		if string(c.AuthJWTSecret) == devSecret {
			r.fail("HP_AUTH_JWT_SECRET must be set to a real secret in production")
		}
		if string(c.FilesURLSecret) == devSecret {
			r.fail("HP_FILES_URL_SECRET must be set to a real secret in production")
		}
	}
	if len(r.problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(r.problems, "\n  - "))
	}
	return c, nil
}

type reader struct {
	getenv   func(string) string
	problems []string
}

func (r *reader) fail(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

func (r *reader) str(key, fallback string) string {
	if v := strings.TrimSpace(r.getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (r *reader) oneOf(key, fallback string, allowed ...string) string {
	v := r.str(key, fallback)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	r.fail("%s must be one of %v, got %q", key, allowed, v)
	return fallback
}

func (r *reader) url(key, fallback string) string {
	v := r.str(key, fallback)
	if v == "" {
		return ""
	}
	if u, err := url.Parse(v); err != nil || u.Scheme == "" || u.Host == "" {
		r.fail("%s must be a URL like scheme://host, got %q", key, redact(v))
	}
	return v
}

func (r *reader) duration(key string, fallback time.Duration) time.Duration {
	v := r.str(key, "")
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.fail("%s must be a duration like 15s, got %q", key, v)
		return fallback
	}
	return d
}

func (r *reader) int64(key string, fallback int64) int64 {
	v := r.str(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		r.fail("%s must be a whole number, got %q", key, v)
		return fallback
	}
	return n
}

func (r *reader) level(key string, fallback slog.Level) slog.Level {
	v := r.str(key, "")
	if v == "" {
		return fallback
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		r.fail("%s must be debug, info, warn or error, got %q", key, v)
		return fallback
	}
	return l
}

// redact hides credentials in URLs before they reach an error message.
func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable>"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.String()
}
