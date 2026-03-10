package httpcapture

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBufferCapture(t *testing.T) {
	tests := []struct {
		name       string
		maxBytes   int
		chunks     [][]byte
		wantBytes  []byte
		wantTotal  int64
		wantCutOff bool
	}{
		{
			name:       "captures bytes within limit",
			maxBytes:   8,
			chunks:     [][]byte{[]byte("he"), []byte("llo")},
			wantBytes:  []byte("hello"),
			wantTotal:  5,
			wantCutOff: false,
		},
		{
			name:       "captures only up to max bytes",
			maxBytes:   4,
			chunks:     [][]byte{[]byte("ab"), []byte("cdef")},
			wantBytes:  []byte("abcd"),
			wantTotal:  6,
			wantCutOff: true,
		},
		{
			name:       "marks zero max bytes as truncated",
			maxBytes:   0,
			chunks:     [][]byte{[]byte("abc")},
			wantBytes:  nil,
			wantTotal:  3,
			wantCutOff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := NewBuffer(tt.maxBytes)

			for _, chunk := range tt.chunks {
				buffer.Capture(chunk)
			}

			if diff := cmp.Diff(tt.wantBytes, buffer.Bytes()); diff != "" {
				t.Fatalf("Bytes mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantTotal, buffer.TotalBytes()); diff != "" {
				t.Fatalf("TotalBytes mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantCutOff, buffer.Truncated()); diff != "" {
				t.Fatalf("Truncated mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBufferBytesReturnsCopy(t *testing.T) {
	buffer := NewBuffer(8)
	buffer.Capture([]byte("hello"))

	got := buffer.Bytes()
	got[0] = 'j'

	if diff := cmp.Diff([]byte("hello"), buffer.Bytes()); diff != "" {
		t.Fatalf("Bytes mismatch after caller mutation (-want +got):\n%s", diff)
	}
}

func TestBufferMarkComplete(t *testing.T) {
	buffer := NewBuffer(4)
	if buffer.Completed() {
		t.Fatal("Completed should be false before MarkComplete")
	}

	buffer.MarkComplete()

	if !buffer.Completed() {
		t.Fatal("Completed should be true after MarkComplete")
	}
}
