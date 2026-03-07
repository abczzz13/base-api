package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/abczzz13/base-api/internal/server"
)

func main() {
	ctx := context.Background()
	if err := server.Run(ctx, os.LookupEnv, os.Stderr); err != nil {
		slog.Error("api exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}
