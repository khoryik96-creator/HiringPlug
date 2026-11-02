// Package clientip carries the address of the client that sent a request, for
// limits on public endpoints (e.g. how many applications one place may send).
//
// Behind a reverse proxy every request comes from the proxy; the proxy then
// names the client in a header it sets itself (nginx: X-Real-IP). Only that
// configured header is trusted (HP_CLIENT_IP_HEADER); a client cannot set it
// because the proxy overwrites it.
package clientip

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type key struct{}

// With returns ctx carrying ip.
func With(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, key{}, ip)
}

// From returns the client address in ctx, or "" when unknown.
func From(ctx context.Context) string {
	ip, _ := ctx.Value(key{}).(string)
	return ip
}

// Of returns r's client address: the trusted header when configured and
// present, else the connection's own address.
func Of(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if v := strings.TrimSpace(r.Header.Get(trustedHeader)); v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
