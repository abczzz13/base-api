package middleware

import (
	"net/http"
	"slices"
)

type Chain struct {
	httpMiddlewares []func(http.Handler) http.Handler
}

func NewChain(middlewares ...func(http.Handler) http.Handler) *Chain {
	return &Chain{
		httpMiddlewares: middlewares,
	}
}

func (c *Chain) With(m func(http.Handler) http.Handler) *Chain {
	c.httpMiddlewares = append(c.httpMiddlewares, m)
	return c
}

func (c *Chain) WrapHandler(h http.Handler) http.Handler {
	for _, mw := range slices.Backward(c.httpMiddlewares) {
		h = mw(h)
	}
	return h
}
