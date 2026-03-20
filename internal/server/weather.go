package server

import (
	"fmt"

	weatherclient "github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/httpclient"
	"github.com/abczzz13/base-api/internal/outboundaudit"
)

func setupWeatherClient(cfg config.Config, httpMetrics *httpclient.Metrics, auditRepo outboundaudit.Repository) (weatherclient.Client, error) {
	geocodingClient, err := httpclient.New(httpclient.Config{
		Client:          "open_meteo_geocoding",
		BaseURL:         cfg.Weather.GeocodingBaseURL,
		Metrics:         httpMetrics,
		AuditRepository: auditRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("create weather geocoding client: %w", err)
	}

	forecastClient, err := httpclient.New(httpclient.Config{
		Client:          "open_meteo_forecast",
		BaseURL:         cfg.Weather.ForecastBaseURL,
		Metrics:         httpMetrics,
		AuditRepository: auditRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("create weather forecast client: %w", err)
	}

	client, err := weatherclient.New(geocodingClient, forecastClient, cfg.Weather.APIKey, cfg.Weather.Timeout)
	if err != nil {
		return nil, fmt.Errorf("configure weather integration: %w", err)
	}

	return client, nil
}
