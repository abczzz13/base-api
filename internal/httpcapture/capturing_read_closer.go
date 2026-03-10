package httpcapture

import (
	"errors"
	"io"
)

// CapturingReadCloser wraps an io.ReadCloser and captures bytes read into an embedded Buffer.
type CapturingReadCloser struct {
	io.ReadCloser
	Buffer
}

// NewCapturingReadCloser creates a CapturingReadCloser that captures up to maxBytes from body.
func NewCapturingReadCloser(body io.ReadCloser, maxBytes int) *CapturingReadCloser {
	return &CapturingReadCloser{
		ReadCloser: body,
		Buffer:     NewBuffer(maxBytes),
	}
}

func (c *CapturingReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		c.Capture(p[:n])
	}
	if errors.Is(err, io.EOF) {
		c.MarkComplete()
	}

	return n, err
}
