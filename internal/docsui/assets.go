package docsui

import _ "embed"

//go:embed index.html
var swaggerUIPage string

//go:embed assets/swagger-ui.css
var swaggerUICSS []byte

//go:embed assets/swagger-ui-bundle.js
var swaggerUIBundleJS []byte

//go:embed assets/swagger-ui-standalone-preset.js
var swaggerUIPresetJS []byte
