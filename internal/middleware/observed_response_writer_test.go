package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureObservedResponseWriterCreatesWrapperWhenMissing(t *testing.T) {
	rec := httptest.NewRecorder()

	nextWriter, observedRW := ensureObservedResponseWriter(rec)

	if observedRW == nil {
		t.Fatal("expected observed response writer to be created")
	}
	if observedRW.ResponseWriter != rec {
		t.Fatalf("expected wrapped response writer to be original recorder")
	}
	if nextWriter != observedRW {
		t.Fatalf("expected next writer to be observed response writer")
	}
}

func TestEnsureObservedResponseWriterReusesExistingWrapper(t *testing.T) {
	rec := httptest.NewRecorder()
	existing := newObservedResponseWriter(rec)

	nextWriter, observedRW := ensureObservedResponseWriter(existing)

	if nextWriter != existing {
		t.Fatalf("expected next writer to stay unchanged")
	}
	if observedRW != existing {
		t.Fatalf("expected existing observed response writer to be reused")
	}
}

func TestEnsureObservedResponseWriterReusesWrappedObservedWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	existing := newObservedResponseWriter(rec)
	wrapped := &unwrapResponseWriter{ResponseWriter: existing}
	wrappedTwice := &unwrapResponseWriter{ResponseWriter: wrapped}

	nextWriter, observedRW := ensureObservedResponseWriter(wrappedTwice)

	if nextWriter != wrappedTwice {
		t.Fatalf("expected next writer to preserve outer wrapper")
	}
	if observedRW != existing {
		t.Fatalf("expected wrapped observed response writer to be reused")
	}
}

func TestObservedResponseWriterTracksFinalStatusAfterInformationalHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newObservedResponseWriter(rec)

	rw.WriteHeader(http.StatusEarlyHints)
	rw.WriteHeader(http.StatusAccepted)

	if rw.statusCode != http.StatusAccepted {
		t.Fatalf("expected tracked status %d, got %d", http.StatusAccepted, rw.statusCode)
	}
}

func TestObservedResponseWriterDefaultsToOKAfterInformationalHeadersWhenBodyWritten(t *testing.T) {
	rw := newObservedResponseWriter(&permissiveResponseWriter{})

	rw.WriteHeader(http.StatusEarlyHints)
	if _, err := rw.Write([]byte("ok")); err != nil {
		t.Fatalf("write response body: %v", err)
	}

	if rw.statusCode != http.StatusOK {
		t.Fatalf("expected tracked status %d, got %d", http.StatusOK, rw.statusCode)
	}
}

func TestObservedResponseWriterTreatsSwitchingProtocolsAsFinalStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newObservedResponseWriter(rec)

	rw.WriteHeader(http.StatusSwitchingProtocols)
	rw.WriteHeader(http.StatusOK)

	if rw.statusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected tracked status %d, got %d", http.StatusSwitchingProtocols, rw.statusCode)
	}
}

type permissiveResponseWriter struct {
	header http.Header
}

func (w *permissiveResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}

	return w.header
}

func (*permissiveResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (*permissiveResponseWriter) WriteHeader(int) {}

type unwrapResponseWriter struct {
	http.ResponseWriter
}

func (w *unwrapResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
