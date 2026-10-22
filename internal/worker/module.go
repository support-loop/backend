package worker

import (
	"github.com/go-core-fx/fxutil"
	"github.com/go-core-fx/logger"
	"go.uber.org/fx"
)

// Module wires the in-process background worker. The queue is created inside
// worker.New; no channel value enters the fx graph. The prometheus.Registerer
// is provided at app level (see internal/commands/serve/serve.go).
func Module(withRun bool) fx.Option {
	opts := []fx.Option{
		logger.WithNamedLogger("worker"),
		fx.Provide(NewMetrics, fx.Private),
		fx.Provide(NewStubHandler, fx.Private),
		fx.Provide(New),
	}
	if withRun {
		opts = append(opts, fx.Invoke(fxutil.RegisterRunnable[*Service]()))
	}
	return fx.Module("worker", opts...)
}
