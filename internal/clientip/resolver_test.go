package clientip

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	baseclientip "github.com/abczzz13/clientip"
	"github.com/google/go-cmp/cmp"
)

type stubExtractor struct {
	calls int
	addr  netip.Addr
	err   error
}

func (s *stubExtractor) ExtractAddr(*http.Request, ...baseclientip.OverrideOptions) (netip.Addr, error) {
	s.calls++
	if s.err != nil {
		return netip.Addr{}, s.err
	}

	return s.addr, nil
}

func TestResolverResolveStrictCachesResult(t *testing.T) {
	extractor := &stubExtractor{addr: netip.MustParseAddr("203.0.113.10")}
	resolver := NewResolverWithExtractor(extractor)
	req := httptest.NewRequest("GET", "/healthz", nil)

	resolvedReq, got := resolver.ResolveStrict(req)
	if diff := cmp.Diff("203.0.113.10", got); diff != "" {
		t.Fatalf("ResolveStrict IP mismatch (-want +got):\n%s", diff)
	}

	strictFromContext, ok := StrictFromContext(resolvedReq.Context())
	if !ok {
		t.Fatal("StrictFromContext should report cached value")
	}
	if diff := cmp.Diff("203.0.113.10", strictFromContext); diff != "" {
		t.Fatalf("StrictFromContext mismatch (-want +got):\n%s", diff)
	}

	resolvedReq, got = resolver.ResolveStrict(resolvedReq)
	if diff := cmp.Diff("203.0.113.10", got); diff != "" {
		t.Fatalf("ResolveStrict cached IP mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(1, extractor.calls); diff != "" {
		t.Fatalf("extractor call count mismatch (-want +got):\n%s", diff)
	}
	if _, ok := PreferredFromContext(resolvedReq.Context()); ok {
		t.Fatal("PreferredFromContext should be unset after strict-only resolution")
	}
}

func TestResolverResolvePreferredFallsBackToRemoteAddrAndCachesResult(t *testing.T) {
	extractor := &stubExtractor{err: errors.New("boom")}
	resolver := NewResolverWithExtractor(extractor)
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.RemoteAddr = "198.51.100.7:43123"

	resolvedReq, got := resolver.ResolvePreferred(req)
	if diff := cmp.Diff("198.51.100.7", got); diff != "" {
		t.Fatalf("ResolvePreferred IP mismatch (-want +got):\n%s", diff)
	}

	strictFromContext, ok := StrictFromContext(resolvedReq.Context())
	if !ok {
		t.Fatal("StrictFromContext should record attempted resolution")
	}
	if diff := cmp.Diff("", strictFromContext); diff != "" {
		t.Fatalf("StrictFromContext mismatch (-want +got):\n%s", diff)
	}

	preferredFromContext, ok := PreferredFromContext(resolvedReq.Context())
	if !ok {
		t.Fatal("PreferredFromContext should report cached preferred value")
	}
	if diff := cmp.Diff("198.51.100.7", preferredFromContext); diff != "" {
		t.Fatalf("PreferredFromContext mismatch (-want +got):\n%s", diff)
	}

	_, got = resolver.ResolvePreferred(resolvedReq)
	if diff := cmp.Diff("198.51.100.7", got); diff != "" {
		t.Fatalf("ResolvePreferred cached IP mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(1, extractor.calls); diff != "" {
		t.Fatalf("extractor call count mismatch (-want +got):\n%s", diff)
	}
}

func TestRequestRemoteClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{
			name:       "extracts IP from host port",
			remoteAddr: "203.0.113.5:8080",
			want:       "203.0.113.5",
		},
		{
			name:       "returns hostname from host port",
			remoteAddr: "api.internal:8080",
			want:       "api.internal",
		},
		{
			name:       "trims bare IP",
			remoteAddr: " 2001:db8::1 ",
			want:       "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = tt.remoteAddr

			if diff := cmp.Diff(tt.want, requestRemoteClientIP(req)); diff != "" {
				t.Fatalf("requestRemoteClientIP mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
