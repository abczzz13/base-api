package middleware

import (
	"net/http"

	"github.com/abczzz13/base-api/internal/requestid"
)

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := requestid.New(r.Header.Get(requestid.HeaderName))
			w.Header().Set(requestid.HeaderName, requestID)
			next.ServeHTTP(w, r.WithContext(requestid.WithContext(r.Context(), requestID)))
		})
	}
}
