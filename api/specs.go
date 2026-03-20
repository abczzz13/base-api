package api

import _ "embed"

var (
	//go:embed openapi.yaml
	publicOpenAPISpecYAML []byte

	//go:embed weather_openapi.yaml
	weatherOpenAPISpecYAML []byte

	//go:embed infra_openapi.yaml
	infraOpenAPISpecYAML []byte
)

func PublicOpenAPISpecYAML() []byte {
	return append([]byte(nil), publicOpenAPISpecYAML...)
}

func WeatherOpenAPISpecYAML() []byte {
	return append([]byte(nil), weatherOpenAPISpecYAML...)
}

func InfraOpenAPISpecYAML() []byte {
	return append([]byte(nil), infraOpenAPISpecYAML...)
}
