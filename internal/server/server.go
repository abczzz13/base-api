package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/version"
)

func Run(
	ctx context.Context,
	args []string,
	getenv func(string) string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	_ = args
	_ = stdin
	_ = stdout

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, configWarnings := loadConfigWithWarnings(getenv)

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

	logConfigWarnings(ctx, configWarnings)

	shutdownTracing := setupTracing(ctx, &cfg)
	defer shutdownTracing()

	runtimeDeps, err := newRuntimeDependencies()
	if err != nil {
		return fmt.Errorf("configure HTTP metrics: %w", err)
	}

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

	errCh := serveServers(servers)

	shutdownAll := func() error {
		defer shutdownTracing()
		return shutdownServers(servers)
	}

	select {
	case res := <-errCh:
		if err := shutdownAll(); err != nil {
			return err
		}
		if res.err != nil {
			return res.err
		}
		return fmt.Errorf("%s server stopped unexpectedly", res.name)
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down")

		if err := shutdownAll(); err != nil {
			return err
		}

		for range servers {
			res := <-errCh
			if res.err != nil {
				return res.err
			}
		}

		return nil
	}
}

func logConfigWarnings(ctx context.Context, warnings []string) {
	for _, warning := range warnings {
		slog.WarnContext(ctx, "config warning", slog.String("warning", warning))
	}
}
