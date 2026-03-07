package outboundaudit

import (
	"context"
	"time"
)

// Repository persists outbound HTTP audit records.
type Repository interface {
	StoreOutboundAudit(context.Context, Record) error
}

// RepositoryFunc adapts a function into Repository.
type RepositoryFunc func(context.Context, Record) error

// StoreOutboundAudit implements Repository.
func (f RepositoryFunc) StoreOutboundAudit(ctx context.Context, record Record) error {
	if f == nil {
		return nil
	}

	return f(ctx, record)
}

// NopRepository returns a Repository that drops records.
func NopRepository() Repository {
	return RepositoryFunc(func(context.Context, Record) error {
		return nil
	})
}

// Record contains a fully redacted outbound HTTP audit event.
type Record struct {
	Client                string
	Operation             string
	Method                string
	Path                  string
	Query                 string
	Host                  string
	Scheme                string
	StatusCode            int
	Duration              time.Duration
	RequestSizeBytes      int64
	ResponseSizeBytes     int64
	RequestHeaders        map[string][]string
	ResponseHeaders       map[string][]string
	RequestBody           []byte
	ResponseBody          []byte
	RequestBodyTruncated  bool
	ResponseBodyTruncated bool
	ErrorKind             string
	ErrorMessage          string
	RequestID             string
	TraceID               string
	SpanID                string
}
