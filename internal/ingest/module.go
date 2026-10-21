package ingest

import (
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the ingest domain. The prometheus.Registerer is provided at
// app level (see internal/commands/serve/serve.go); NewMetrics consumes it
// via DI. The metrics are exported (not fx.Private): the webhook HTTP handler
// counts the auth failures that happen in its keyauth middleware, so it needs
// the same metrics instance.
func Module() fx.Option {
	return fx.Module(
		"ingest",
		logger.WithNamedLogger("ingest"),
		fx.Provide(NewMetrics),
		fx.Provide(New),
	)
}
