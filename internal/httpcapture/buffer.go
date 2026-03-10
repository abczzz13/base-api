package httpcapture

import "bytes"

// Buffer captures bytes up to a maximum size, tracking total bytes and truncation.
type Buffer struct {
	maxBytes   int
	buffer     bytes.Buffer
	totalBytes int64
	truncated  bool
	completed  bool
}

// NewBuffer creates a capture buffer with the given maximum byte limit.
func NewBuffer(maxBytes int) Buffer {
	return Buffer{maxBytes: maxBytes}
}

// Capture writes a chunk of bytes into the buffer, respecting the size limit.
func (cb *Buffer) Capture(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	cb.totalBytes += int64(len(chunk))

	if cb.maxBytes <= 0 {
		cb.truncated = true
		return
	}

	remaining := cb.maxBytes - cb.buffer.Len()
	if remaining <= 0 {
		cb.truncated = true
		return
	}

	if len(chunk) > remaining {
		_, _ = cb.buffer.Write(chunk[:remaining])
		cb.truncated = true
		return
	}

	_, _ = cb.buffer.Write(chunk)
}

// Bytes returns a copy of the captured bytes.
func (cb *Buffer) Bytes() []byte {
	return append([]byte(nil), cb.buffer.Bytes()...)
}

// Truncated reports whether the captured data was truncated.
func (cb *Buffer) Truncated() bool {
	return cb.truncated
}

// TotalBytes returns the total number of bytes observed (including those beyond the limit).
func (cb *Buffer) TotalBytes() int64 {
	return cb.totalBytes
}

// MarkComplete marks the capture as having seen all data (e.g., on EOF).
func (cb *Buffer) MarkComplete() {
	cb.completed = true
}

// Completed reports whether MarkComplete has been called.
func (cb *Buffer) Completed() bool {
	return cb.completed
}
