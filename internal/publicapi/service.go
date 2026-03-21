package publicapi

import (
	"context"
)

// HealthResponse is the handwritten public health payload.
type HealthResponse struct {
	Status string
}

// Service contains handwritten public API behavior, independent of generated transport types.
type Service struct{}

// NewService creates a new public API service.
func NewService() *Service {
	return &Service{}
}

func (s *Service) GetHealthz(context.Context) (HealthResponse, error) {
	return HealthResponse{Status: "OK"}, nil
}
