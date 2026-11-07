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
