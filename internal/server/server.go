package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/version"
)

func Run(
	ctx context.Context,
	args []string,
	lookupEnv func(string) (string, bool),
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	_ = args
	_ = stdin
	_ = stdout

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load(lookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger.New(logger.Config{
		Format:      cfg.LogFormat,
		Level:       cfg.LogLevel,
		Version:     version.GetVersion(),
		Environment: cfg.Environment,
		Writer:      stderr,
	})

	buildMetadata := version.GetBuildMetadata()
	slog.InfoContext(
		ctx,
		"build metadata",
		slog.String("version", buildMetadata.Version),
		slog.String("git_commit", buildMetadata.GitCommit),
		slog.String("git_commit_short", buildMetadata.GitCommitShort),
		slog.String("git_tag", buildMetadata.GitTag),
		slog.String("git_branch", buildMetadata.GitBranch),
		slog.String("git_tree_state", buildMetadata.GitTreeState),
		slog.String("build_time", buildMetadata.BuildTime),
		slog.String("go_version", buildMetadata.GoVersion),
	)

	runtimeCleanup := newCleanupStack()
	defer runtimeCleanup.Run()

	tracingEnabled, shutdownTracing := setupTracing(ctx, cfg)
	cfg.OTEL.TracingEnabled = tracingEnabled
	runtimeCleanup.Add(shutdownTracing)

	runtimeDeps, err := newRuntimeDependencies()
	if err != nil {
		return fmt.Errorf("configure HTTP metrics: %w", err)
	}

	database, databaseShutdown, err := setupDatabase(ctx, cfg.DB, runtimeDeps.metricsRegisterer)
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	runtimeDeps.database = database
	runtimeCleanup.Add(databaseShutdown)

	publicHandler, err := newPublicHandler(cfg, runtimeDeps)
	if err != nil {
		return fmt.Errorf("create public API handler: %w", err)
	}

	infraHandler, err := newInfraHandler(cfg, runtimeDeps)
	if err != nil {
		return fmt.Errorf("create infra API handler: %w", err)
	}

	servers := []managedServer{
		{
			name:   "public",
			server: newHTTPServer(cfg, cfg.Address, publicHandler),
		},
		{
			name:   "infra",
			server: newHTTPServer(cfg, cfg.InfraAddress, infraHandler),
		},
	}

	if err := bindServerListeners(ctx, servers); err != nil {
		return err
	}

	serverResults := runServers(servers)

	shutdownAll := func() error {
		shutdownErr := shutdownServers(servers)
		runtimeCleanup.Run()
		return shutdownErr
	}

	select {
	case res := <-serverResults:
		serveErr := res.err
		if serveErr == nil {
			serveErr = fmt.Errorf("%s server stopped unexpectedly", res.name)
		}

		return errors.Join(serveErr, shutdownAll())
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down")

		if err := shutdownAll(); err != nil {
			return err
		}

		for range servers {
			res := <-serverResults
			if res.err != nil {
				return res.err
			}
		}

		return nil
	}
}
