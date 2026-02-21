package api

import _ "embed"

var (
	//go:embed openapi.yaml
	publicOpenAPISpecYAML []byte

	//go:embed infra_openapi.yaml
	infraOpenAPISpecYAML []byte
)

func PublicOpenAPISpecYAML() []byte {
	return append([]byte(nil), publicOpenAPISpecYAML...)
}

func InfraOpenAPISpecYAML() []byte {
	return append([]byte(nil), infraOpenAPISpecYAML...)
}
