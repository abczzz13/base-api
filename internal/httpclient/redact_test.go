package httpclient

import (
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRedactHeadersSanitizesURLLikeValues(t *testing.T) {
	t.Parallel()

	headers := http.Header{
		"Location":    {"https://example.com/callback?token=secret"},
		"Link":        {`<https://example.com/page?cursor=secret>; rel="next"`},
		"X-Callback":  {"before https://example.com/hook?sig=secret after"},
		"X-Proxy-URL": {"//example.com/internal?token=secret"},
	}

	got := redactHeaders(headers)
	want := map[string][]string{
		"Location":    {"[REDACTED_URL]"},
		"Link":        {`<[REDACTED_URL]>; rel="next"`},
		"X-Callback":  {"before [REDACTED_URL] after"},
		"X-Proxy-URL": {"[REDACTED_URL]"},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("redactHeaders mismatch (-want +got):\n%s", diff)
	}
}

func TestRedactHeadersKeepsSensitiveHeadersFullyRedacted(t *testing.T) {
	t.Parallel()

	headers := http.Header{
		"Authorization": {"Bearer secret"},
		"X-API-Key":     {"secret-key"},
	}

	got := redactHeaders(headers)
	want := map[string][]string{
		"Authorization": {redactedValue},
		"X-API-Key":     {redactedValue},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("redactHeaders mismatch (-want +got):\n%s", diff)
	}
}
