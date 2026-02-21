package docsui

import "net/http"

const (
	infraOpenAPISpecPath           = "/openapi/infra.yaml"
	publicOpenAPISpecPath          = "/openapi/public.yaml"
	swaggerPath                    = "/swagger"
	swaggerUICSSPath               = "/swagger-ui/swagger-ui.css"
	swaggerUIBundleJSPath          = "/swagger-ui/swagger-ui-bundle.js"
	swaggerUIPresetJSPath          = "/swagger-ui/swagger-ui-standalone-preset.js"
	docsPath                       = "/docs"
	docsSlashPath                  = "/docs/"
	openAPISpecCacheTTL            = "public, max-age=300"
	swaggerPageCacheTTL            = "public, max-age=300"
	swaggerAssetCacheTTL           = "public, max-age=300"
	swaggerUIContentSecurityPolicy = "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; font-src 'self' data:; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

	getInfraOpenAPISpecPattern  = http.MethodGet + " " + infraOpenAPISpecPath
	getPublicOpenAPISpecPattern = http.MethodGet + " " + publicOpenAPISpecPath
	getSwaggerPattern           = http.MethodGet + " " + swaggerPath
	getSwaggerUICSSPattern      = http.MethodGet + " " + swaggerUICSSPath
	getSwaggerUIBundlePattern   = http.MethodGet + " " + swaggerUIBundleJSPath
	getSwaggerUIPresetPattern   = http.MethodGet + " " + swaggerUIPresetJSPath
	getDocsPattern              = http.MethodGet + " " + docsPath
	getDocsSlashPattern         = http.MethodGet + " " + docsSlashPath + "{$}"
)

// Register installs docs UI and OpenAPI routes into mux.
func Register(mux *http.ServeMux) {
	mux.HandleFunc(getPublicOpenAPISpecPattern, handlePublicOpenAPISpec)
	mux.HandleFunc(getInfraOpenAPISpecPattern, handleInfraOpenAPISpec)
	mux.HandleFunc(getSwaggerPattern, handleSwaggerUI)
	mux.HandleFunc(getSwaggerUICSSPattern, handleSwaggerUICSS)
	mux.HandleFunc(getSwaggerUIBundlePattern, handleSwaggerUIBundleJS)
	mux.HandleFunc(getSwaggerUIPresetPattern, handleSwaggerUIPresetJS)
	mux.HandleFunc(getDocsPattern, handleDocsRedirect)
	mux.HandleFunc(getDocsSlashPattern, handleDocsRedirect)
}
