package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/postgres"
)

func TestRunIgnoresLogWriteErrors(t *testing.T) {
	dbURL := requireDatabaseURLAndReachable(t)
	assertRunHandlesWriterErrors(t, dbURL, errWriter{err: errors.New("stderr unavailable")})
}

func TestRunClosesBoundListenersWhenLaterListenFails(t *testing.T) {
	dbURL := requireDatabaseURLAndReachable(t)

	publicAddr := reserveTCPAddress(t)

	occupiedInfraListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for occupied infra address: %v", err)
	}
	defer func() { _ = occupiedInfraListener.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = Run(
		ctx,
		lookupEnvFromMap(map[string]string{
			"API_ADDR":        publicAddr,
			"API_INFRA_ADDR":  occupiedInfraListener.Addr().String(),
			"API_ENVIRONMENT": "test",
			"DB_URL":          dbURL,
		}),
		io.Discard,
	)
	if err == nil {
		t.Fatalf("Run returned nil error when infra listen should fail")
	}
	if !strings.Contains(err.Error(), "create infra listener") {
		t.Fatalf("Run error does not identify infra listener failure: %v", err)
	}

	releasedListener, err := net.Listen("tcp", publicAddr)
	if err != nil {
		t.Fatalf("public listener address was not released after startup failure: %v", err)
	}
	_ = releasedListener.Close()
}

func TestRunContinuesWithInvalidTracingExporterConfig(t *testing.T) {
	dbURL := requireDatabaseURLAndReachable(t)

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "://invalid-endpoint")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "://invalid-endpoint")

	publicAddr := reserveTCPAddress(t)
	infraAddr := reserveTCPAddress(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stderr bytes.Buffer
	env := map[string]string{
		"API_ADDR":                    publicAddr,
		"API_INFRA_ADDR":              infraAddr,
		"API_ENVIRONMENT":             "test",
		"DB_URL":                      dbURL,
		"OTEL_EXPORTER_OTLP_ENDPOINT": "://invalid-endpoint",
	}

	runDone := make(chan struct{})
	var runErr error
	go func() {
		runErr = Run(ctx, lookupEnvFromMap(env), &stderr)
		close(runDone)
	}()

	if err := waitForStatusOK("http://"+publicAddr+"/healthz", runDone, &runErr); err != nil {
		t.Fatalf("public server startup check failed: %v", err)
	}
	if err := waitForStatusOK("http://"+infraAddr+"/livez", runDone, &runErr); err != nil {
		t.Fatalf("infra server startup check failed: %v", err)
	}

	publicResp, err := http.Get("http://" + publicAddr + "/healthz")
	if err != nil {
		t.Fatalf("public health request failed: %v", err)
	}
	defer func() { _ = publicResp.Body.Close() }()
	if publicResp.StatusCode != http.StatusOK {
		t.Fatalf("public health status mismatch: want %d, got %d", http.StatusOK, publicResp.StatusCode)
	}

	cancel()
	if !waitForRunDone(runDone, 3*time.Second) {
		t.Fatal("Run did not return after cancellation")
	}

	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
}

type errWriter struct {
	err error
}

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

const maxRunStartupAttempts = 5

func assertRunHandlesWriterErrors(t *testing.T, dbURL string, stderr io.Writer) {
	t.Helper()

	var lastErr error
	for attempt := 1; attempt <= maxRunStartupAttempts; attempt++ {
		publicAddr := reserveTCPAddress(t)
		infraAddr := reserveTCPAddress(t)

		ctx, cancel := context.WithCancel(context.Background())
		env := map[string]string{
			"API_ADDR":        publicAddr,
			"API_INFRA_ADDR":  infraAddr,
			"API_ENVIRONMENT": "test",
			"DB_URL":          dbURL,
		}

		runDone := make(chan struct{})
		var runErr error
		go func() {
			runErr = Run(ctx, lookupEnvFromMap(env), stderr)
			close(runDone)
		}()

		startupErr := waitForStatusOK("http://"+publicAddr+"/healthz", runDone, &runErr)
		if startupErr == nil {
			startupErr = waitForStatusOK("http://"+infraAddr+"/livez", runDone, &runErr)
		}

		if startupErr != nil {
			cancel()
			if !waitForRunDone(runDone, 3*time.Second) {
				t.Fatalf("Run did not return after failed startup attempt")
			}

			if isAddressInUseError(startupErr) && attempt < maxRunStartupAttempts {
				lastErr = startupErr
				continue
			}

			t.Fatalf("startup check failed: %v", startupErr)
		}

		cancel()
		if !waitForRunDone(runDone, 3*time.Second) {
			t.Fatalf("Run did not return after cancellation")
		}

		if runErr != nil {
			if isAddressInUseError(runErr) && attempt < maxRunStartupAttempts {
				lastErr = runErr
				continue
			}

			t.Fatalf("Run returned error: %v", runErr)
		}

		return
	}

	t.Fatalf("Run failed after %d startup attempts: %v", maxRunStartupAttempts, lastErr)
}

func waitForStatusOK(url string, runDone <-chan struct{}, runErr *error) error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-runDone:
			if *runErr != nil {
				return fmt.Errorf("run exited before %s became ready: %w", url, *runErr)
			}
			return fmt.Errorf("run exited before %s became ready", url)
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	select {
	case <-runDone:
		if *runErr != nil {
			return fmt.Errorf("run exited before %s became ready: %w", url, *runErr)
		}
		return fmt.Errorf("run exited before %s became ready", url)
	default:
	}

	return fmt.Errorf("timed out waiting for %s", url)
}

func waitForRunDone(runDone <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-runDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

func isAddressInUseError(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

func requireDatabaseURLAndReachable(t *testing.T) string {
	t.Helper()

	dbURL := testDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := postgres.Open(ctx, config.DBConfig{
		URL:               dbURL,
		MinConns:          0,
		MaxConns:          1,
		MaxConnLifetime:   time.Minute,
		MaxConnIdleTime:   30 * time.Second,
		HealthCheckPeriod: time.Second,
		ConnectTimeout:    2 * time.Second,
	})
	if err != nil {
		t.Skipf("PostgreSQL integration unavailable: %v", err)
	}
	pool.Close()

	return dbURL
}
