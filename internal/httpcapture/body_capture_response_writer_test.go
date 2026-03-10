package httpcapture

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestBodyCaptureResponseWriterTracksWrites(t *testing.T) {
	rw := NewBodyCaptureResponseWriter(&permissiveResponseWriter{}, 4)

	if _, err := rw.Write([]byte("hello")); err != nil {
		t.Fatalf("write body: %v", err)
	}

	if got, want := string(rw.Bytes()), "hell"; got != want {
		t.Fatalf("captured bytes mismatch: want %q, got %q", want, got)
	}
	if got, want := rw.TotalBytes(), int64(5); got != want {
		t.Fatalf("captured byte count mismatch: want %d, got %d", want, got)
	}
	if !rw.Truncated() {
		t.Fatal("expected capture to be truncated")
	}
}

func TestBodyCaptureResponseWriterReadFromCapturesBody(t *testing.T) {
	underlying := &readerFromResponseWriter{}
	rw := NewBodyCaptureResponseWriter(underlying, 5)

	if _, err := io.Copy(rw, bytes.NewBufferString("payload")); err != nil {
		t.Fatalf("copy response body: %v", err)
	}

	if got, want := underlying.String(), "payload"; got != want {
		t.Fatalf("underlying body mismatch: want %q, got %q", want, got)
	}
	if got, want := string(rw.Bytes()), "paylo"; got != want {
		t.Fatalf("captured bytes mismatch: want %q, got %q", want, got)
	}
	if got, want := rw.TotalBytes(), int64(len("payload")); got != want {
		t.Fatalf("captured byte count mismatch: want %d, got %d", want, got)
	}
	if !rw.Truncated() {
		t.Fatal("expected capture to be truncated")
	}
}

type readerFromResponseWriter struct {
	header http.Header
	bytes.Buffer
}

func (w *readerFromResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}

	return w.header
}

func (w *readerFromResponseWriter) WriteHeader(int) {}

func (w *readerFromResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(&w.Buffer, r)
}
