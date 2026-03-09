package clientip

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/abczzz13/clientip"
)

// Extractor extracts a client IP address from an HTTP request.
type Extractor interface {
	ExtractAddr(*http.Request, ...clientip.OverrideOptions) (netip.Addr, error)
}

type contextKey struct{}

type contextValue struct {
	strictResolved    bool
	strictValue       string
	preferredResolved bool
	preferredValue    string
}

// Resolver resolves and caches the canonical client IP for a request.
type Resolver struct {
	extractor Extractor
}

// NewResolver creates a shared client IP resolver.
func NewResolver(component string, trustedProxyCIDRs []netip.Prefix) *Resolver {
	ext, err := newStrictExtractor(trustedProxyCIDRs)
	if err != nil {
		slog.Warn(component+" client IP extractor initialization failed", slog.Any("error", err))
	}

	return &Resolver{extractor: ext}
}

// NewResolverWithExtractor creates a resolver with a custom extractor, useful for testing.
func NewResolverWithExtractor(ext Extractor) *Resolver {
	return &Resolver{extractor: ext}
}

func (r *Resolver) ResolveStrict(req *http.Request) (*http.Request, string) {
	if req == nil {
		return nil, ""
	}

	if cached, ok := StrictFromContext(req.Context()); ok {
		return req, cached
	}

	var ext Extractor
	if r != nil {
		ext = r.extractor
	}

	ip := strings.TrimSpace(requestClientIP(req, ext))

	return withStrictClientIP(req, ip), ip
}

func (r *Resolver) ResolvePreferred(req *http.Request) (*http.Request, string) {
	if req == nil {
		return nil, ""
	}

	if cached, ok := PreferredFromContext(req.Context()); ok {
		return req, cached
	}

	req, ip := r.ResolveStrict(req)
	if ip == "" {
		ip = strings.TrimSpace(requestRemoteClientIP(req))
	}

	return withPreferredClientIP(req, ip), ip
}

// StrictFromContext returns the strict client IP from the request context, if resolved.
func StrictFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value, ok := ctx.Value(contextKey{}).(contextValue)
	if !ok || !value.strictResolved {
		return "", false
	}

	return value.strictValue, true
}

// PreferredFromContext returns the preferred client IP from the request context, if resolved.
func PreferredFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value, ok := ctx.Value(contextKey{}).(contextValue)
	if !ok || !value.preferredResolved {
		return "", false
	}

	return value.preferredValue, true
}

func withStrictClientIP(req *http.Request, ip string) *http.Request {
	if req == nil {
		return nil
	}

	value, _ := req.Context().Value(contextKey{}).(contextValue)
	value.strictResolved = true
	value.strictValue = ip

	ctx := context.WithValue(req.Context(), contextKey{}, value)

	return req.WithContext(ctx)
}

func withPreferredClientIP(req *http.Request, ip string) *http.Request {
	if req == nil {
		return nil
	}

	value, _ := req.Context().Value(contextKey{}).(contextValue)
	value.preferredResolved = true
	value.preferredValue = ip

	ctx := context.WithValue(req.Context(), contextKey{}, value)

	return req.WithContext(ctx)
}

func newStrictExtractor(trustedProxyCIDRs []netip.Prefix) (*clientip.Extractor, error) {
	opts := []clientip.Option{
		clientip.Priority(clientip.SourceXForwardedFor, clientip.SourceRemoteAddr),
		clientip.WithSecurityMode(clientip.SecurityModeStrict),
		clientip.AllowPrivateIPs(true),
	}

	if len(trustedProxyCIDRs) == 0 {
		opts = append([]clientip.Option{clientip.TrustLocalProxyDefaults()}, opts...)
	} else {
		opts = append([]clientip.Option{clientip.TrustProxyPrefixes(trustedProxyCIDRs...)}, opts...)
	}

	ext, err := clientip.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("initialize client IP extractor: %w", err)
	}

	return ext, nil
}

func requestClientIP(r *http.Request, ext Extractor) string {
	if ext == nil {
		return requestRemoteClientIP(r)
	}

	extractedIP, err := ext.ExtractAddr(r)
	if err != nil {
		return ""
	}

	return extractedIP.String()
}

func requestRemoteClientIP(r *http.Request) string {
	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		if parsed := net.ParseIP(host); parsed != nil {
			return parsed.String()
		}

		return host
	}

	if parsed := net.ParseIP(remoteAddr); parsed != nil {
		return parsed.String()
	}

	return remoteAddr
}
