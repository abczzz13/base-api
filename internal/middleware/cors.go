package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowedMethods := cfg.AllowedMethods
	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}
	}

	allowedHeaders := cfg.AllowedHeaders
	if len(allowedHeaders) == 0 {
		allowedHeaders = []string{"Content-Type", "Authorization", "Accept"}
	}

	maxAge := cfg.MaxAge
	if maxAge == 0 {
		maxAge = 5 * time.Minute
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addVary(w.Header(), "Origin")

			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodOptions {
				addVary(w.Header(), "Access-Control-Request-Method", "Access-Control-Request-Headers")
			}

			if len(cfg.AllowedOrigins) == 0 {
				observeCORSPolicyDenial(r)

				next.ServeHTTP(w, r)
				return
			}

			allowed := isOriginAllowed(origin, cfg.AllowedOrigins, cfg.AllowCredentials)
			if !allowed {
				observeCORSPolicyDenial(r)

				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)

			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if len(cfg.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", "))
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(maxAge.Seconds())))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isOriginAllowed(origin string, allowedOrigins []string, allowCredentials bool) bool {
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			if allowCredentials {
				continue
			}
			return true
		}
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

func addVary(header http.Header, values ...string) {
	present := make(map[string]struct{})
	for _, existing := range header.Values("Vary") {
		for _, part := range strings.Split(existing, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			present[strings.ToLower(trimmed)] = struct{}{}
		}
	}

	for _, value := range values {
		key := strings.ToLower(value)
		if _, exists := present[key]; exists {
			continue
		}

		header.Add("Vary", value)
		present[key] = struct{}{}
	}
}
