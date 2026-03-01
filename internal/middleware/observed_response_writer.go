package middleware

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

type observedResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	written      bool
}

func newObservedResponseWriter(w http.ResponseWriter) *observedResponseWriter {
	return &observedResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func ensureObservedResponseWriter(w http.ResponseWriter) (http.ResponseWriter, *observedResponseWriter) {
	if observedRW, ok := findObservedResponseWriter(w); ok {
		return w, observedRW
	}

	observedRW := newObservedResponseWriter(w)
	return observedRW, observedRW
}

func findObservedResponseWriter(w http.ResponseWriter) (*observedResponseWriter, bool) {
	type responseWriterUnwrapper interface {
		Unwrap() http.ResponseWriter
	}

	current := w
	for current != nil {
		if observedRW, ok := current.(*observedResponseWriter); ok {
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

func (rw *observedResponseWriter) WriteHeader(code int) {
	if isInformationalStatus(code) {
		rw.ResponseWriter.WriteHeader(code)
		return
	}

	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}

	rw.ResponseWriter.WriteHeader(code)
}

func (rw *observedResponseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}

	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func (rw *observedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}

	if readerFrom, ok := rw.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(r)
		rw.bytesWritten += n
		return n, err
	}

	n, err := io.Copy(rw.ResponseWriter, r)
	rw.bytesWritten += n
	return n, err
}

func (rw *observedResponseWriter) Flush() {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}

	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *observedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	return hijacker.Hijack()
}

func (rw *observedResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := rw.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, opts)
}

func (rw *observedResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func isInformationalStatus(code int) bool {
	return code >= http.StatusContinue && code < http.StatusOK && code != http.StatusSwitchingProtocols
}
