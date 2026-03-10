package httpcapture

import (
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCapturingReadCloserCapturesBytes(t *testing.T) {
	body := io.NopCloser(strings.NewReader("hello world"))
	capture := NewCapturingReadCloser(body, 5)

	got, err := io.ReadAll(capture)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if diff := cmp.Diff("hello world", string(got)); diff != "" {
		t.Fatalf("read content mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]byte("hello"), capture.Bytes()); diff != "" {
		t.Fatalf("captured bytes mismatch (-want +got):\n%s", diff)
	}
	if !capture.Truncated() {
		t.Fatal("expected capture to be truncated")
	}
	if diff := cmp.Diff(int64(11), capture.TotalBytes()); diff != "" {
		t.Fatalf("TotalBytes mismatch (-want +got):\n%s", diff)
	}
	if !capture.Completed() {
		t.Fatal("expected capture to be completed after EOF")
	}
}

func TestCapturingReadCloserWithinLimit(t *testing.T) {
	body := io.NopCloser(strings.NewReader("hi"))
	capture := NewCapturingReadCloser(body, 10)

	got, err := io.ReadAll(capture)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if diff := cmp.Diff("hi", string(got)); diff != "" {
		t.Fatalf("read content mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]byte("hi"), capture.Bytes()); diff != "" {
		t.Fatalf("captured bytes mismatch (-want +got):\n%s", diff)
	}
	if capture.Truncated() {
		t.Fatal("expected capture not to be truncated")
	}
	if !capture.Completed() {
		t.Fatal("expected capture to be completed after EOF")
	}
}

func TestCapturingReadCloserClose(t *testing.T) {
	body := io.NopCloser(strings.NewReader("data"))
	capture := NewCapturingReadCloser(body, 10)

	if err := capture.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
