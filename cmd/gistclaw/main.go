// cmd/gistclaw/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config: load failed")
	}

	// Configure zerolog: JSON to stdout (structured, journalctl-parseable)
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	zerolog.SetGlobalLevel(level)
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

	a, err := app.NewApp(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("app: init failed")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("app: run failed")
	}
}
