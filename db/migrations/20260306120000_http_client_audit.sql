-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS http_client_audit (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client TEXT NOT NULL,
    operation TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL DEFAULT '',
    query TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    scheme TEXT NOT NULL DEFAULT '',
    status_code INTEGER NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL,
    request_size_bytes BIGINT NOT NULL,
    response_size_bytes BIGINT NOT NULL,
    request_headers JSONB NOT NULL,
    response_headers JSONB NOT NULL,
    request_body JSONB,
    response_body JSONB,
    request_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    error_kind TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    span_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_http_client_audit_status_code_range CHECK (status_code = 0 OR status_code BETWEEN 100 AND 599),
    CONSTRAINT chk_http_client_audit_duration_ms_non_negative CHECK (duration_ms >= 0),
    CONSTRAINT chk_http_client_audit_request_size_non_negative CHECK (request_size_bytes >= 0),
    CONSTRAINT chk_http_client_audit_response_size_non_negative CHECK (response_size_bytes >= 0),
    CONSTRAINT chk_http_client_audit_request_headers_object CHECK (jsonb_typeof(request_headers) = 'object'),
    CONSTRAINT chk_http_client_audit_response_headers_object CHECK (jsonb_typeof(response_headers) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_http_client_audit_created_at ON http_client_audit (created_at);
CREATE INDEX IF NOT EXISTS idx_http_client_audit_client_created_at ON http_client_audit (client, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_http_client_audit_trace_id ON http_client_audit (trace_id) WHERE trace_id <> '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS http_client_audit;
-- +goose StatementEnd
