package requestaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/abczzz13/base-api/internal/dbsqlc"
)

type postgresRepository struct {
	queries *dbsqlc.Queries
}

// NewPostgresRepository creates a sqlc-backed request-audit repository.
func NewPostgresRepository(database dbsqlc.DBTX) Repository {
	if database == nil {
		return NopRepository()
	}

	return &postgresRepository{
		queries: dbsqlc.New(database),
	}
}

func (repo *postgresRepository) StoreRequestAudit(ctx context.Context, record Record) error {
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

	clientIP := parseClientIP(record.ClientIP)

	return repo.queries.InsertHTTPRequestAudit(ctx, dbsqlc.InsertHTTPRequestAuditParams{
		Server:                record.Server,
		Route:                 record.Route,
		Method:                record.Method,
		Path:                  record.Path,
		Query:                 record.Query,
		Host:                  record.Host,
		Scheme:                record.Scheme,
		Proto:                 record.Proto,
		StatusCode:            normalizeStatusCode(record.StatusCode),
		DurationMs:            durationMs,
		RequestSizeBytes:      normalizeSizeBytes(record.RequestSizeBytes),
		ResponseSizeBytes:     normalizeSizeBytes(record.ResponseSizeBytes),
		RemoteAddr:            record.RemoteAddr,
		ClientIp:              clientIP,
		UserAgent:             record.UserAgent,
		RequestHeaders:        requestHeaders,
		ResponseHeaders:       responseHeaders,
		RequestBody:           jsonColumn(record.RequestBody),
		ResponseBody:          jsonColumn(record.ResponseBody),
		RequestBodyTruncated:  record.RequestBodyTruncated,
		ResponseBodyTruncated: record.ResponseBodyTruncated,
		TraceID:               record.TraceID,
		SpanID:                record.SpanID,
	})
}

func marshalHeaders(headers map[string][]string) ([]byte, error) {
	if headers == nil {
		return []byte("{}"), nil
	}

	encoded, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("marshal request audit headers: %w", err)
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
	if value < http.StatusContinue {
		return http.StatusInternalServerError
	}
	if value > 599 {
		return 599
	}

	return int32(value)
}

func normalizeSizeBytes(value int64) int64 {
	if value < 0 {
		return 0
	}

	return value
}

func parseClientIP(value string) *netip.Addr {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	parsed, err := netip.ParseAddr(trimmed)
	if err != nil {
		return nil
	}

	unmapped := parsed.Unmap()
	return &unmapped
}
