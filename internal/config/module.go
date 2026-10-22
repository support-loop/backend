package config

import (
	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/fiberfx/openapi"
	"github.com/go-core-fx/sqlfx"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/worker"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"config",
		fx.Provide(New, fx.Private),
		fx.Provide(
			func(cfg Config) fiberfx.Config {
				return fiberfx.Config{
					Address:     cfg.HTTP.Address,
					ProxyHeader: cfg.HTTP.ProxyHeader,
					Proxies:     cfg.HTTP.Proxies,
				}
			},
			func(cfg Config) openapi.Config {
				return openapi.Config{
					Enabled:    cfg.HTTP.OpenAPI.Enabled,
					PublicHost: cfg.HTTP.OpenAPI.PublicHost,
					PublicPath: cfg.HTTP.OpenAPI.PublicPath,
				}
			},
			func(cfg Config) sqlfx.Config {
				return sqlfx.Config{
					URL:             cfg.Database.URL,
					ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
					ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
					MaxOpenConns:    cfg.Database.MaxOpenConns,
					MaxIdleConns:    cfg.Database.MaxIdleConns,
				}
			},
			func(cfg Config) ingest.Config {
				return ingest.Config{
					Secret:        cfg.Webhook.Secret,
					SecretHeader:  cfg.Webhook.SecretHeader,
					AllowedEvents: cfg.Webhook.AllowedEvents,
				}
			},
			func(cfg Config) worker.Config {
				return worker.Config{
					QueueSize: cfg.Worker.QueueSize,
				}
			},
		),
	)
}
