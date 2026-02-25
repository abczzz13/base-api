package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChain(t *testing.T) {
	t.Run("chains middlewares in order", func(t *testing.T) {
		var order []string

		middleware1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m1-before")
				next.ServeHTTP(w, r)
				order = append(order, "m1-after")
			})
		}

		middleware2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m2-before")
				next.ServeHTTP(w, r)
				order = append(order, "m2-after")
			})
		}

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
			w.WriteHeader(http.StatusOK)
		})

		chain := NewChain(middleware1, middleware2)
		wrapped := chain.WrapHandler(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
		if len(order) != len(expected) {
			t.Errorf("expected %d calls, got %d", len(expected), len(order))
		}
		for i, v := range expected {
			if i >= len(order) || order[i] != v {
				t.Errorf("expected order[%d] = %q, got %q", i, v, order[i])
			}
		}
	})

	t.Run("With adds middleware to chain", func(t *testing.T) {
		var called bool

		middleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				next.ServeHTTP(w, r)
			})
		}

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		chain := NewChain().With(middleware)
		wrapped := chain.WrapHandler(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if !called {
			t.Error("expected middleware to be called")
		}
	})
}
