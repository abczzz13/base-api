package httpcapture

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureObservedResponseWriterCreatesWrapperWhenMissing(t *testing.T) {
	rec := httptest.NewRecorder()

	nextWriter, observedRW := EnsureObservedResponseWriter(rec)

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
	existing := NewObservedResponseWriter(rec)

	nextWriter, observedRW := EnsureObservedResponseWriter(existing)

	if nextWriter != existing {
		t.Fatalf("expected next writer to stay unchanged")
	}
	if observedRW != existing {
		t.Fatalf("expected existing observed response writer to be reused")
	}
}

func TestEnsureObservedResponseWriterReusesWrappedObservedWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	existing := NewObservedResponseWriter(rec)
	wrapped := &unwrapResponseWriter{ResponseWriter: existing}
	wrappedTwice := &unwrapResponseWriter{ResponseWriter: wrapped}

	nextWriter, observedRW := EnsureObservedResponseWriter(wrappedTwice)

	if nextWriter != wrappedTwice {
		t.Fatalf("expected next writer to preserve outer wrapper")
	}
	if observedRW != existing {
		t.Fatalf("expected wrapped observed response writer to be reused")
	}
}

func TestObservedResponseWriterTracksFinalStatusAfterInformationalHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewObservedResponseWriter(rec)

	rw.WriteHeader(http.StatusEarlyHints)
	rw.WriteHeader(http.StatusAccepted)

	if rw.StatusCode != http.StatusAccepted {
		t.Fatalf("expected tracked status %d, got %d", http.StatusAccepted, rw.StatusCode)
	}
}

func TestObservedResponseWriterDefaultsToOKAfterInformationalHeadersWhenBodyWritten(t *testing.T) {
	rw := NewObservedResponseWriter(&permissiveResponseWriter{})

	rw.WriteHeader(http.StatusEarlyHints)
	if _, err := rw.Write([]byte("ok")); err != nil {
		t.Fatalf("write response body: %v", err)
	}

	if rw.StatusCode != http.StatusOK {
		t.Fatalf("expected tracked status %d, got %d", http.StatusOK, rw.StatusCode)
	}
}

func TestObservedResponseWriterTreatsSwitchingProtocolsAsFinalStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewObservedResponseWriter(rec)

	rw.WriteHeader(http.StatusSwitchingProtocols)
	rw.WriteHeader(http.StatusOK)

	if rw.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected tracked status %d, got %d", http.StatusSwitchingProtocols, rw.StatusCode)
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
