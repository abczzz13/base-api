package requestaudit

import (
	"context"
	"net/netip"
	"strings"

	"github.com/abczzz13/base-api/internal/asyncaudit"
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

	requestHeaders, err := asyncaudit.MarshalHeaders(record.RequestHeaders)
	if err != nil {
		return err
	}

	responseHeaders, err := asyncaudit.MarshalHeaders(record.ResponseHeaders)
	if err != nil {
		return err
	}

	durationMs := max(record.Duration.Milliseconds(), 0)

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
		RequestSizeBytes:      asyncaudit.NormalizeSizeBytes(record.RequestSizeBytes),
		ResponseSizeBytes:     asyncaudit.NormalizeSizeBytes(record.ResponseSizeBytes),
		RemoteAddr:            record.RemoteAddr,
		ClientIp:              clientIP,
		UserAgent:             record.UserAgent,
		RequestHeaders:        requestHeaders,
		ResponseHeaders:       responseHeaders,
		RequestBody:           asyncaudit.JSONColumn(record.RequestBody),
		ResponseBody:          asyncaudit.JSONColumn(record.ResponseBody),
		RequestBodyTruncated:  record.RequestBodyTruncated,
		ResponseBodyTruncated: record.ResponseBodyTruncated,
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
