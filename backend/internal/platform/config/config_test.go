package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaultsAreUsableForDevelopment(t *testing.T) {
	c, err := Load(env(map[string]string{"HP_DATABASE_URL": "postgres://u:p@localhost:5432/hp"}))
	require.NoError(t, err)
	assert.Equal(t, EnvDevelopment, c.Env)
	assert.Equal(t, ":8080", c.HTTPAddr)
	assert.Equal(t, "MY", c.DefaultPhoneArea)
	assert.Equal(t, "Asia/Kuala_Lumpur", c.TimeZone.String())
	assert.EqualValues(t, 10<<20, c.MaxUploadBytes)
	assert.Equal(t, 4, c.WorkerConcurrency)
}

func TestEveryProblemIsReportedAtOnce(t *testing.T) {
	_, err := Load(env(map[string]string{
		"HP_ENV":                "staging",
		"HP_LOG_LEVEL":          "loud",
		"HP_MAX_UPLOAD_BYTES":   "lots",
		"HP_AUTH_JWT_SECRET":    "short",
		"HP_TIME_ZONE":          "Mars/Base",
		"HP_WORKER_CONCURRENCY": "0",
	}))
	require.Error(t, err)
	for _, want := range []string{"HP_ENV", "HP_LOG_LEVEL", "HP_MAX_UPLOAD_BYTES", "HP_AUTH_JWT_SECRET", "HP_DATABASE_URL", "HP_TIME_ZONE", "HP_WORKER_CONCURRENCY"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestProductionRefusesDevelopmentSecrets(t *testing.T) {
	_, err := Load(env(map[string]string{"HP_ENV": "production", "HP_DATABASE_URL": "postgres://u:p@db:5432/hp"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HP_AUTH_JWT_SECRET must be set to a real secret")

	// Every published default is refused, the mail keys and the database
	// password among them (the Docker file ships with them).
	real := "a-real-secret-0123456789-0123456789-xyz"
	_, err = Load(env(map[string]string{
		"HP_ENV": "production", "HP_DATABASE_URL": "postgres://hp_api:hp_api_dev@db:5432/hp",
		"HP_AUTH_JWT_SECRET": real, "HP_FILES_URL_SECRET": real, "HP_MAIL_KEYS_SECRET": devSecret,
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HP_MAIL_KEYS_SECRET must be a real secret")
	assert.Contains(t, err.Error(), "HP_DATABASE_URL must not use the published development password")
	assert.NotContains(t, err.Error(), "hp_api_dev")

	_, err = Load(env(map[string]string{
		"HP_ENV": "production", "HP_DATABASE_URL": "postgres://hp_api:s3cretpw@db:5432/hp",
		"HP_AUTH_JWT_SECRET": real, "HP_FILES_URL_SECRET": real, "HP_MAIL_KEYS_SECRET": real,
		"HP_PUBLIC_URL": "https://app.example.com",
	}))
	require.NoError(t, err)
}

func TestProductionRefusesTheLaptopsAddressAndMailCatcher(t *testing.T) {
	real := "a-real-secret-0123456789-0123456789-xyz"
	base := map[string]string{
		"HP_ENV": "production", "HP_DATABASE_URL": "postgres://hp_api:s3cretpw@db:5432/hp",
		"HP_AUTH_JWT_SECRET": real, "HP_FILES_URL_SECRET": real, "HP_MAIL_KEYS_SECRET": real,
	}
	with := func(extra map[string]string) error {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		_, err := Load(env(m))
		return err
	}
	for _, address := range []string{"", "https://localhost:3000", "http://127.0.0.1:3000"} {
		err := with(map[string]string{"HP_PUBLIC_URL": address})
		require.Error(t, err, address)
		assert.Contains(t, err.Error(), "HP_PUBLIC_URL must be the address others open the app at", address)
	}
	err := with(map[string]string{"HP_PUBLIC_URL": "http://app.example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HP_PUBLIC_URL must be the app's https:// address", "on the internet: https")
	err = with(map[string]string{"HP_PUBLIC_URL": "https://app.example.com", "HP_SMTP_HOST": "mailpit:1025", "HP_SMTP_FROM": "no-reply@example.com", "HP_SMTP_TLS": "none"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HP_SMTP_HOST must be your mail server in production")
	err = with(map[string]string{"HP_PUBLIC_URL": "https://app.example.com", "HP_SMTP_HOST": "smtp.example.com:587", "HP_SMTP_FROM": "no-reply@example.com", "HP_SMTP_TLS": "none"})
	require.Error(t, err, "real mail is sent encrypted")
	require.NoError(t, with(map[string]string{"HP_PUBLIC_URL": "https://app.example.com", "HP_SMTP_HOST": "smtp.example.com:587", "HP_SMTP_FROM": "no-reply@example.com"}))
	// A server in the office, reached by its network address, keeps working as
	// it did: plain http, and whatever email it used.
	for _, address := range []string{"http://192.168.1.10:3000", "http://10.0.0.5", "http://hiring-server:3000", "http://hp.office.lan"} {
		require.NoError(t, with(map[string]string{"HP_PUBLIC_URL": address, "HP_SMTP_HOST": "mailpit:1025", "HP_SMTP_FROM": "no-reply@example.com", "HP_SMTP_TLS": "none"}), address)
	}
	// Outside production the laptop's settings stay fine.
	_, err = Load(env(map[string]string{"HP_DATABASE_URL": "postgres://u:p@localhost:5432/hp", "HP_SMTP_HOST": "mailpit:1025", "HP_SMTP_FROM": "no-reply@example.com", "HP_SMTP_TLS": "none"}))
	require.NoError(t, err)
}

func TestPasswordsNeverAppearInErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"HP_DATABASE_URL": "postgres://user:hunter2@/nohost"}))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestMailServerSettings(t *testing.T) {
	c, err := Load(env(map[string]string{"HP_DATABASE_URL": "postgres://u:p@localhost:5432/hp"}))
	require.NoError(t, err)
	assert.Empty(t, c.SMTP.Addr, "no mail server unless one is set")
	assert.Equal(t, "starttls", c.SMTP.TLS, "encrypted by default")

	_, err = Load(env(map[string]string{
		"HP_DATABASE_URL": "postgres://u:p@localhost:5432/hp", "HP_SMTP_HOST": "smtp.example.com",
		"HP_SMTP_FROM": "HiringPlug <x@example.com>", "HP_SMTP_TLS": "maybe", "HP_SMTP_PASSWORD": "hunter2",
	}))
	require.Error(t, err)
	for _, want := range []string{"HP_SMTP_HOST must be host:port", "HP_SMTP_FROM must be an email address", "HP_SMTP_TLS"} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestAlertEmailsAreAddresses(t *testing.T) {
	c, err := Load(env(map[string]string{"HP_DATABASE_URL": "postgres://u:p@localhost:5432/hp", "HP_ALERT_EMAIL": " you@agency.com, it@agency.com "}))
	require.NoError(t, err)
	assert.Equal(t, []string{"you@agency.com", "it@agency.com"}, c.AlertEmails)
	_, err = Load(env(map[string]string{"HP_DATABASE_URL": "postgres://u:p@localhost:5432/hp", "HP_ALERT_EMAIL": "you at agency"}))
	assert.ErrorContains(t, err, "HP_ALERT_EMAIL must be email addresses")
}
