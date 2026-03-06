package requestaudit

import (
	"context"
	"time"
)

// Repository persists request-audit records.
type Repository interface {
	StoreRequestAudit(context.Context, Record) error
}

// RepositoryFunc adapts a function into Repository.
type RepositoryFunc func(context.Context, Record) error

// StoreRequestAudit implements Repository.
func (f RepositoryFunc) StoreRequestAudit(ctx context.Context, record Record) error {
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

// Record contains a fully redacted request-audit event.
type Record struct {
	Server                string
	Route                 string
	Method                string
	Path                  string
	Query                 string
	Host                  string
	Scheme                string
	Proto                 string
	StatusCode            int
	Duration              time.Duration
	RequestSizeBytes      int64
	ResponseSizeBytes     int64
	RemoteAddr            string
	ClientIP              string
	UserAgent             string
	RequestHeaders        map[string][]string
	ResponseHeaders       map[string][]string
	RequestBody           []byte
	ResponseBody          []byte
	RequestBodyTruncated  bool
	ResponseBodyTruncated bool
	TraceID               string
	SpanID                string
}
