package httpclient

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const (
	redactedValue     = "[REDACTED]"
	invalidQueryValue = "_redacted=invalid_query"
)

var (
	invalidJSONPlaceholder   = []byte(`{"_redacted":"invalid_json"}`)
	truncatedJSONPlaceholder = []byte(`{"_redacted":"truncated"}`)
	nonJSONPlaceholder       = []byte(`{"_redacted":"non_json"}`)
	headerURLPattern         = regexp.MustCompile(`(?:https?://|//)[^\s"'<>]+`)

	sensitiveJSONKeys = map[string]struct{}{
		"password":      {},
		"passphrase":    {},
		"secret":        {},
		"clientsecret":  {},
		"token":         {},
		"accesstoken":   {},
		"refreshtoken":  {},
		"idtoken":       {},
		"apikey":        {},
		"privatekey":    {},
		"credential":    {},
		"credentials":   {},
		"session":       {},
		"sessionid":     {},
		"authorization": {},
		"cookie":        {},
		"setcookie":     {},
		"jwt":           {},
	}
)

func redactQuery(rawQuery string) string {
	trimmed := strings.TrimSpace(rawQuery)
	if trimmed == "" {
		return ""
	}

	values, err := url.ParseQuery(trimmed)
	if err != nil {
		return invalidQueryValue
	}

	for key, entries := range values {
		if !sensitiveJSONKey(key) {
			continue
		}

		values[key] = redactedValues(len(entries))
	}

	return values.Encode()
}

func redactHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return map[string][]string{}
	}

	result := make(map[string][]string, len(headers))
	for key, values := range headers {
		if sensitiveHeader(key) {
			result[key] = redactedValues(len(values))
			continue
		}

		result[key] = sanitizeHeaderValues(values)
	}

	return result
}

func sanitizeHeaderValues(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	result := make([]string, len(values))
	for i, value := range values {
		result[i] = sanitizeHeaderValue(value)
	}

	return result
}

func sanitizeHeaderValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}

	return headerURLPattern.ReplaceAllString(value, "[REDACTED_URL]")
}

func redactBody(body []byte, truncated bool, contentType string) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}

	if truncated {
		return append([]byte(nil), truncatedJSONPlaceholder...)
	}

	if shouldTreatAsNonJSON(contentType) {
		return append([]byte(nil), nonJSONPlaceholder...)
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return append([]byte(nil), invalidJSONPlaceholder...)
	}

	marshaled, err := json.Marshal(redactJSONValue(value))
	if err != nil {
		return append([]byte(nil), invalidJSONPlaceholder...)
	}

	return marshaled
}

func shouldTreatAsNonJSON(contentType string) bool {
	trimmed := strings.TrimSpace(contentType)
	if trimmed == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return false
	}

	normalized := strings.ToLower(strings.TrimSpace(mediaType))
	if normalized == "application/json" {
		return false
	}

	return !strings.HasSuffix(normalized, "+json")
}

func redactJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, nestedValue := range typed {
			if sensitiveJSONKey(key) {
				result[key] = redactedValue
				continue
			}

			result[key] = redactJSONValue(nestedValue)
		}

		return result
	case []any:
		result := make([]any, len(typed))
		for i, nestedValue := range typed {
			result[i] = redactJSONValue(nestedValue)
		}

		return result
	default:
		return value
	}
}

func sensitiveJSONKey(key string) bool {
	normalized := normalizeKey(key)
	if normalized == "" {
		return false
	}

	if _, ok := sensitiveJSONKeys[normalized]; ok {
		return true
	}

	if strings.HasSuffix(normalized, "password") ||
		strings.HasSuffix(normalized, "passphrase") ||
		strings.HasSuffix(normalized, "secret") ||
		strings.HasSuffix(normalized, "token") ||
		strings.HasSuffix(normalized, "apikey") ||
		strings.HasSuffix(normalized, "privatekey") ||
		strings.HasSuffix(normalized, "credential") ||
		strings.HasSuffix(normalized, "credentials") ||
		strings.HasSuffix(normalized, "session") ||
		strings.HasSuffix(normalized, "sessionid") ||
		strings.HasSuffix(normalized, "jwt") {
		return true
	}

	return false
}

func sensitiveHeader(headerName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(headerName))
	if normalized == "" {
		return false
	}

	switch normalized {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}

	if strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.HasSuffix(normalized, "-key") ||
		strings.HasSuffix(normalized, "_key") {
		return true
	}

	return false
}

func redactedValues(count int) []string {
	if count <= 0 {
		return []string{redactedValue}
	}

	values := make([]string, count)
	for i := range values {
		values[i] = redactedValue
	}

	return values
}

func normalizeKey(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(unicode.ToLower(character))
		}
	}

	return builder.String()
}
