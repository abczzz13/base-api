package responsewriter

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// ObservedResponseWriter wraps an http.ResponseWriter to track status code and bytes written.
type ObservedResponseWriter struct {
	http.ResponseWriter
	StatusCode   int
	BytesWritten int64
	written      bool
}

// NewObservedResponseWriter creates a new observed response writer.
func NewObservedResponseWriter(w http.ResponseWriter) *ObservedResponseWriter {
	return &ObservedResponseWriter{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
}

// EnsureObservedResponseWriter returns an existing ObservedResponseWriter from the chain,
// or wraps the given writer in a new one.
func EnsureObservedResponseWriter(w http.ResponseWriter) (http.ResponseWriter, *ObservedResponseWriter) {
	if observedRW, ok := FindObservedResponseWriter(w); ok {
		return w, observedRW
	}

	observedRW := NewObservedResponseWriter(w)
	return observedRW, observedRW
}

// FindObservedResponseWriter searches the response writer chain for an ObservedResponseWriter.
func FindObservedResponseWriter(w http.ResponseWriter) (*ObservedResponseWriter, bool) {
	type responseWriterUnwrapper interface {
		Unwrap() http.ResponseWriter
	}

	current := w
	for current != nil {
		if observedRW, ok := current.(*ObservedResponseWriter); ok {
			return observedRW, true
		}

		unwrapper, ok := current.(responseWriterUnwrapper)
		if !ok {
			return nil, false
		}

		next := unwrapper.Unwrap()
		if next == nil || next == current {
			return nil, false
		}

		current = next
	}

	return nil, false
}

func (rw *ObservedResponseWriter) WriteHeader(code int) {
	if isInformationalStatus(code) {
		rw.ResponseWriter.WriteHeader(code)
		return
	}

	if !rw.written {
		rw.StatusCode = code
		rw.written = true
	}

	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ObservedResponseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}

	n, err := rw.ResponseWriter.Write(b)
	rw.BytesWritten += int64(n)
	return n, err
}

func (rw *ObservedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}

	if readerFrom, ok := rw.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(r)
		rw.BytesWritten += n
		return n, err
	}

	n, err := io.Copy(rw.ResponseWriter, r)
	rw.BytesWritten += n
	return n, err
}

func (rw *ObservedResponseWriter) Flush() {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}

	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *ObservedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	return hijacker.Hijack()
}

func (rw *ObservedResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := rw.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, opts)
}

func (rw *ObservedResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func isInformationalStatus(code int) bool {
	return code >= http.StatusContinue && code < http.StatusOK && code != http.StatusSwitchingProtocols
}
