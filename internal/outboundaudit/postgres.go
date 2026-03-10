package outboundaudit

import (
	"context"
	"strings"

	"github.com/abczzz13/base-api/internal/asyncaudit"
	"github.com/abczzz13/base-api/internal/dbsqlc"
)

type postgresRepository struct {
	queries *dbsqlc.Queries
}

// NewPostgresRepository creates a sqlc-backed outbound-audit repository.
func NewPostgresRepository(database dbsqlc.DBTX) Repository {
	if database == nil {
		return NopRepository()
	}

	return &postgresRepository{
		queries: dbsqlc.New(database),
	}
}

func (repo *postgresRepository) StoreOutboundAudit(ctx context.Context, record Record) error {
	if repo == nil || repo.queries == nil {
		return nil
	}

	requestHeaders, err := asyncaudit.MarshalHeaders(record.RequestHeaders)
	if err != nil {
		return err
	}

	responseHeaders, err := asyncaudit.MarshalHeaders(record.ResponseHeaders)
	if err != nil {
		return err
	}

	durationMs := max(record.Duration.Milliseconds(), 0)

	return repo.queries.InsertHTTPClientAudit(ctx, dbsqlc.InsertHTTPClientAuditParams{
		Client:                strings.TrimSpace(record.Client),
		Operation:             strings.TrimSpace(record.Operation),
		Method:                strings.TrimSpace(record.Method),
		Path:                  record.Path,
		Query:                 record.Query,
		Host:                  record.Host,
		Scheme:                record.Scheme,
		StatusCode:            normalizeStatusCode(record.StatusCode),
		DurationMs:            durationMs,
		RequestSizeBytes:      asyncaudit.NormalizeSizeBytes(record.RequestSizeBytes),
		ResponseSizeBytes:     asyncaudit.NormalizeSizeBytes(record.ResponseSizeBytes),
		RequestHeaders:        requestHeaders,
		ResponseHeaders:       responseHeaders,
		RequestBody:           asyncaudit.JSONColumn(record.RequestBody),
		ResponseBody:          asyncaudit.JSONColumn(record.ResponseBody),
		RequestBodyTruncated:  record.RequestBodyTruncated,
		ResponseBodyTruncated: record.ResponseBodyTruncated,
		ErrorKind:             strings.TrimSpace(record.ErrorKind),
		ErrorMessage:          strings.TrimSpace(record.ErrorMessage),
		RequestID:             strings.TrimSpace(record.RequestID),
		TraceID:               strings.TrimSpace(record.TraceID),
		SpanID:                strings.TrimSpace(record.SpanID),
	})
}

func normalizeStatusCode(value int) int32 {
	if value < 100 {
		return 0
	}
	if value > 599 {
		return 599
	}

	return int32(value)
}
