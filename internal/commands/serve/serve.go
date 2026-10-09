package serve

import (
	"context"
	"fmt"

	"github.com/go-core-fx/bunfx"
	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/goosefx"
	"github.com/go-core-fx/healthfx"
	"github.com/go-core-fx/logger"
	"github.com/go-core-fx/sqlfx"
	"github.com/go-core-fx/validatorfx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/support-loop/backend/internal/config"
	"github.com/support-loop/backend/internal/db"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/server"
	"github.com/support-loop/backend/internal/worker"
	"github.com/urfave/cli/v3"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Command returns the serve command that starts the full application.
func Command(version healthfx.Version) *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the HTTP server, Telegram bot, and all services",
		Action: func(ctx context.Context, _ *cli.Command) error {
			return run(ctx, version)
		},
	}
}

func run(ctx context.Context, version healthfx.Version) error {
	app := fx.New(
		// CORE MODULES
		logger.Module(),
		logger.WithFxDefaultLogger(),
		// badgerfx.Module(),
		bunfx.Module(),
		// cachefx.Module(),
		fiberfx.Module(),
		// gocqlfx.Module(),
		// gocqlxfx.Module(),
		goosefx.Module(),
		// gormfx.Module(),
		healthfx.Module(),
		// httpfx.Module(),
		// openaifx.Module(),
		// openrouterfx.Module(),
		// redisfx.Module(),
		sqlfx.Module(),
		// sqlxfx.Module(),
		// telegofx.Module(true),
		validatorfx.Module(),
		// watermillfx.Module(),
		//
		// APP MODULES
		fx.Provide(func() prometheus.Registerer {
			return prometheus.DefaultRegisterer
		}),
		config.Module(),
		db.Module(),
		server.Module(),
		//
		// BUSINESS MODULES
		ingest.Module(),
		worker.Module(true),
		fx.Supply(version),

		fx.Invoke(func(lc fx.Lifecycle, logger *zap.Logger) {
			lc.Append(fx.Hook{
				OnStart: func(_ context.Context) error {
					logger.Info("app started")
					return nil
				},
				OnStop: func(_ context.Context) error {
					logger.Info("app stopped")
					return nil
				},
			})
		}),
	)

	startCtx, cancelStart := context.WithTimeout(ctx, app.StartTimeout())
	defer cancelStart()

	if err := app.Start(startCtx); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	select {
	case <-ctx.Done():
	case <-app.Done():
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), app.StopTimeout())
	defer cancelStop()

	if err := app.Stop(stopCtx); err != nil {
		return fmt.Errorf("failed to stop app: %w", err)
	}

	return nil
}
