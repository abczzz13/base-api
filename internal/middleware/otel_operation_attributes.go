package middleware

import (
	ogenmiddleware "github.com/ogen-go/ogen/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func OTELOperationAttributes() ogenmiddleware.Middleware {
	return func(req ogenmiddleware.Request, next ogenmiddleware.Next) (ogenmiddleware.Response, error) {
		if req.Context != nil {
			span := trace.SpanFromContext(req.Context)
			if span.IsRecording() {
				if spanName := operationSpanName(req); spanName != "" {
					span.SetName(spanName)
				}

				attrs := make([]attribute.KeyValue, 0, 3)
				if req.OperationID != "" {
					attrs = append(attrs, attribute.String("api.operation.id", req.OperationID))
				}
				if req.OperationName != "" {
					attrs = append(attrs, attribute.String("api.operation.name", req.OperationName))
				}
				if req.OperationSummary != "" {
					attrs = append(attrs, attribute.String("api.operation.summary", req.OperationSummary))
				}
				if len(attrs) > 0 {
					span.SetAttributes(attrs...)
				}
			}
		}

		return next(req)
	}
}

func operationSpanName(req ogenmiddleware.Request) string {
	operation := req.OperationID
	if operation == "" {
		operation = req.OperationName
	}

	if operation == "" {
		return ""
	}

	if req.Raw != nil && req.Raw.Method != "" {
		return req.Raw.Method + " " + operation
	}

	return operation
}
