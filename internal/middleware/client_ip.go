package middleware

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

type clientIPExtractor interface {
	ExtractAddr(*http.Request, ...clientip.OverrideOptions) (netip.Addr, error)
}

type clientIPContextKey struct{}

type clientIPContextValue struct {
	strictResolved    bool
	strictValue       string
	preferredResolved bool
	preferredValue    string
}

// ClientIPResolver resolves and caches the canonical client IP for a request.
type ClientIPResolver struct {
	extractor clientIPExtractor
}

// NewClientIPResolver creates a shared client IP resolver.
func NewClientIPResolver(component string, trustedProxyCIDRs []netip.Prefix) *ClientIPResolver {
	extractor, err := newStrictClientIPExtractor(trustedProxyCIDRs)
	if err != nil {
		slog.Warn(component+" client IP extractor initialization failed", slog.Any("error", err))
	}

	return &ClientIPResolver{extractor: extractor}
}

func (r *ClientIPResolver) ResolveStrict(req *http.Request) (*http.Request, string) {
	if req == nil {
		return nil, ""
	}

	if cached, ok := strictClientIPFromContext(req.Context()); ok {
		return req, cached
	}

	var extractor clientIPExtractor
	if r != nil {
		extractor = r.extractor
	}

	clientIP := strings.TrimSpace(requestClientIP(req, extractor))

	return withStrictClientIP(req, clientIP), clientIP
}

func (r *ClientIPResolver) ResolvePreferred(req *http.Request) (*http.Request, string) {
	if req == nil {
		return nil, ""
	}

	if cached, ok := preferredClientIPFromContext(req.Context()); ok {
		return req, cached
	}

	req, clientIP := r.ResolveStrict(req)
	if clientIP == "" {
		clientIP = strings.TrimSpace(requestRemoteClientIP(req))
	}

	return withPreferredClientIP(req, clientIP), clientIP
}

func strictClientIPFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value, ok := ctx.Value(clientIPContextKey{}).(clientIPContextValue)
	if !ok || !value.strictResolved {
		return "", false
	}

	return value.strictValue, true
}

func preferredClientIPFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value, ok := ctx.Value(clientIPContextKey{}).(clientIPContextValue)
	if !ok || !value.preferredResolved {
		return "", false
	}

	return value.preferredValue, true
}

func withStrictClientIP(req *http.Request, clientIP string) *http.Request {
	if req == nil {
		return nil
	}

	value, _ := req.Context().Value(clientIPContextKey{}).(clientIPContextValue)
	value.strictResolved = true
	value.strictValue = clientIP

	ctx := context.WithValue(req.Context(), clientIPContextKey{}, value)

	return req.WithContext(ctx)
}

func withPreferredClientIP(req *http.Request, clientIP string) *http.Request {
	if req == nil {
		return nil
	}

	value, _ := req.Context().Value(clientIPContextKey{}).(clientIPContextValue)
	value.preferredResolved = true
	value.preferredValue = clientIP

	ctx := context.WithValue(req.Context(), clientIPContextKey{}, clientIPContextValue{
		strictResolved:    value.strictResolved,
		strictValue:       value.strictValue,
		preferredResolved: value.preferredResolved,
		preferredValue:    value.preferredValue,
	})

	return req.WithContext(ctx)
}

func newStrictClientIPExtractor(trustedProxyCIDRs []netip.Prefix) (*clientip.Extractor, error) {
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

	extractor, err := clientip.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("initialize client IP extractor: %w", err)
	}

	return extractor, nil
}

func requestClientIP(r *http.Request, extractor clientIPExtractor) string {
	if r == nil {
		return ""
	}

	if extractor == nil {
		return requestRemoteClientIP(r)
	}

	extractedIP, err := extractor.ExtractAddr(r)
	if err != nil {
		return ""
	}

	return extractedIP.String()
}

func requestRemoteClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

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
