package outboundaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

	requestHeaders, err := marshalHeaders(record.RequestHeaders)
	if err != nil {
		return err
	}

	responseHeaders, err := marshalHeaders(record.ResponseHeaders)
	if err != nil {
		return err
	}

	durationMs := record.Duration.Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

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
		RequestSizeBytes:      normalizeSizeBytes(record.RequestSizeBytes),
		ResponseSizeBytes:     normalizeSizeBytes(record.ResponseSizeBytes),
		RequestHeaders:        requestHeaders,
		ResponseHeaders:       responseHeaders,
		RequestBody:           jsonColumn(record.RequestBody),
		ResponseBody:          jsonColumn(record.ResponseBody),
		RequestBodyTruncated:  record.RequestBodyTruncated,
		ResponseBodyTruncated: record.ResponseBodyTruncated,
		ErrorKind:             strings.TrimSpace(record.ErrorKind),
		ErrorMessage:          strings.TrimSpace(record.ErrorMessage),
		TraceID:               strings.TrimSpace(record.TraceID),
		SpanID:                strings.TrimSpace(record.SpanID),
	})
}

func marshalHeaders(headers map[string][]string) ([]byte, error) {
	if headers == nil {
		return []byte("{}"), nil
	}

	encoded, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("marshal outbound audit headers: %w", err)
	}

	return encoded, nil
}

func jsonColumn(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}

	return value
}

func normalizeStatusCode(value int) int32 {
	if value <= 0 {
		return 0
	}
	if value > 599 {
		return 599
	}
	if value < 100 {
		return 0
	}

	return int32(value)
}

func normalizeSizeBytes(value int64) int64 {
	if value < 0 {
		return 0
	}

	return value
}
