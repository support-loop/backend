package worker

import (
	"context"

	"go.uber.org/zap"
)

// NewStubHandler returns the T2 stub event handler: it logs the event and
// returns success. Real work (retrieval, LLM, OmniDesk) lands later.
func NewStubHandler(logger *zap.Logger) JobHandler {
	return func(_ context.Context, ev Event) error {
		logger.Info("processing event",
			zap.String("event_type", ev.EventType),
			zap.Int64("case_id", ev.CaseID),
		)
		return nil
	}
}
