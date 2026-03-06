package middleware

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
)

type bodyCaptureResponseWriter struct {
	http.ResponseWriter
	maxBytes   int
	buffer     bytes.Buffer
	totalBytes int64
	truncated  bool
}

func newBodyCaptureResponseWriter(w http.ResponseWriter, maxBytes int) *bodyCaptureResponseWriter {
	return &bodyCaptureResponseWriter{
		ResponseWriter: w,
		maxBytes:       maxBytes,
	}
}

func (capture *bodyCaptureResponseWriter) WriteHeader(statusCode int) {
	capture.ResponseWriter.WriteHeader(statusCode)
}

func (capture *bodyCaptureResponseWriter) Write(body []byte) (int, error) {
	capture.capture(body)
	return capture.ResponseWriter.Write(body)
}

func (capture *bodyCaptureResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	wrappedReader := &bodyCaptureReader{
		Reader:  reader,
		capture: capture.capture,
	}

	if readerFrom, ok := capture.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(wrappedReader)
	}

	return io.Copy(capture.ResponseWriter, wrappedReader)
}

func (capture *bodyCaptureResponseWriter) Flush() {
	if flusher, ok := capture.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (capture *bodyCaptureResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := capture.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	return hijacker.Hijack()
}

func (capture *bodyCaptureResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := capture.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, opts)
}

func (capture *bodyCaptureResponseWriter) Unwrap() http.ResponseWriter {
	return capture.ResponseWriter
}

func (capture *bodyCaptureResponseWriter) Bytes() []byte {
	return append([]byte(nil), capture.buffer.Bytes()...)
}

func (capture *bodyCaptureResponseWriter) Truncated() bool {
	return capture.truncated
}

func (capture *bodyCaptureResponseWriter) TotalBytes() int64 {
	return capture.totalBytes
}

func (capture *bodyCaptureResponseWriter) capture(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	capture.totalBytes += int64(len(chunk))

	if capture.maxBytes <= 0 {
		capture.truncated = true
		return
	}

	remaining := capture.maxBytes - capture.buffer.Len()
	if remaining <= 0 {
		capture.truncated = true
		return
	}

	if len(chunk) > remaining {
		_, _ = capture.buffer.Write(chunk[:remaining])
		capture.truncated = true
		return
	}

	_, _ = capture.buffer.Write(chunk)
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
