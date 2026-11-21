package config

import (
	"net/url"
	"regexp"
	"strings"
)

// Apollo configures a deployment's reviewed CV Studio-compatible broker.
type Apollo struct {
	BrokerOrigin string
	KeysSecret   []byte
}

var apolloWorkerHost = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9][a-z0-9-]*\.workers\.dev$`)

func (r *reader) apollo() Apollo {
	origin := strings.TrimSuffix(r.str("HP_APOLLO_BROKER_URL", ""), "/")
	secret := r.str("HP_APOLLO_KEYS_SECRET", "")
	if origin == "" {
		return Apollo{}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || !apolloWorkerHost.MatchString(u.Host) || origin != "https://"+u.Host {
		r.fail("HP_APOLLO_BROKER_URL must be the HTTPS origin of the approved workers.dev broker")
	}
	if len(secret) < 32 || strings.HasPrefix(secret, "dev-only-") {
		r.fail("HP_APOLLO_KEYS_SECRET must be a dedicated random secret of at least 32 characters when Apollo is enabled")
	}
	return Apollo{BrokerOrigin: origin, KeysSecret: []byte(secret)}
}
