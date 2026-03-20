package docsui

import (
	"io"
	"net/http"

	apiassets "github.com/abczzz13/base-api/api"
)

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", swaggerPageCacheTTL)
	w.Header().Set("Content-Security-Policy", swaggerUIContentSecurityPolicy)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, swaggerUIPage)
}

func writeOpenAPISpec(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", openAPISpecCacheTTL)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeSwaggerAsset(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", swaggerAssetCacheTTL)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func handlePublicOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	writeOpenAPISpec(w, apiassets.PublicOpenAPISpecYAML())
}

func handleWeatherOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	writeOpenAPISpec(w, apiassets.WeatherOpenAPISpecYAML())
}

func handleInfraOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	writeOpenAPISpec(w, apiassets.InfraOpenAPISpecYAML())
}

func handleSwaggerUICSS(w http.ResponseWriter, _ *http.Request) {
	writeSwaggerAsset(w, "text/css; charset=utf-8", swaggerUICSS)
}

func handleSwaggerUIBundleJS(w http.ResponseWriter, _ *http.Request) {
	writeSwaggerAsset(w, "text/javascript; charset=utf-8", swaggerUIBundleJS)
}

func handleSwaggerUIPresetJS(w http.ResponseWriter, _ *http.Request) {
	writeSwaggerAsset(w, "text/javascript; charset=utf-8", swaggerUIPresetJS)
}

func handleDocsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, swaggerPath, http.StatusTemporaryRedirect)
}
