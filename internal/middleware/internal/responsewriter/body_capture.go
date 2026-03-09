package responsewriter

import (
	"bufio"
	"io"
	"net"
	"net/http"

	"github.com/abczzz13/base-api/internal/bodycapture"
)

// BodyCaptureResponseWriter wraps an http.ResponseWriter to capture the response body.
type BodyCaptureResponseWriter struct {
	http.ResponseWriter
	bodycapture.Buffer
}

// NewBodyCaptureResponseWriter creates a new body capture response writer.
func NewBodyCaptureResponseWriter(w http.ResponseWriter, maxBytes int) *BodyCaptureResponseWriter {
	return &BodyCaptureResponseWriter{
		ResponseWriter: w,
		Buffer:         bodycapture.NewBuffer(maxBytes),
	}
}

func (capture *BodyCaptureResponseWriter) Write(body []byte) (int, error) {
	capture.Capture(body)
	return capture.ResponseWriter.Write(body)
}

func (capture *BodyCaptureResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	wrappedReader := &bodyCaptureReader{
		Reader:  reader,
		capture: capture.Capture,
	}

	if readerFrom, ok := capture.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(wrappedReader)
	}

	return io.Copy(capture.ResponseWriter, wrappedReader)
}

func (capture *BodyCaptureResponseWriter) Flush() {
	if flusher, ok := capture.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (capture *BodyCaptureResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := capture.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	return hijacker.Hijack()
}

func (capture *BodyCaptureResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := capture.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, opts)
}

func (capture *BodyCaptureResponseWriter) Unwrap() http.ResponseWriter {
	return capture.ResponseWriter
}

type bodyCaptureReader struct {
	io.Reader
	capture func([]byte)
}

func (reader *bodyCaptureReader) Read(p []byte) (int, error) {
	n, err := reader.Reader.Read(p)
	if n > 0 {
		reader.capture(p[:n])
	}

	return n, err
}
