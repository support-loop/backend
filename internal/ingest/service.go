package ingest

import (
	"context"
	"fmt"

	"github.com/support-loop/backend/internal/worker"
	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// Service validates webhook events, deduplicates them and hands them to the
// worker via its public Enqueue method.
type Service struct {
	config    Config
	db        *bun.DB
	workerSvc *worker.Service
	metrics   *Metrics
	logger    *zap.Logger
}

// New creates the ingest service.
func New(config Config, db *bun.DB, workerSvc *worker.Service, metrics *Metrics, logger *zap.Logger) *Service {
	return &Service{
		config:    config,
		db:        db,
		workerSvc: workerSvc,
		metrics:   metrics,
		logger:    logger,
	}
}

// Process validates the event and enqueues it through the worker service. It
// returns ErrEventNotAllowed for unknown event types and wraps
// worker.ErrChannelFull when the queue is full (the dedup marker is deleted
// first so OmniDesk can retry the event later).
func (s *Service) Process(ctx context.Context, ev worker.Event) (EnqueueResult, error) {
	if !s.config.IsAllowed(ev.EventType) {
		s.metrics.IncUnknownEvent()
		s.metrics.IncReceived(ev.EventType, "ignored")
		return EnqueueResult{}, fmt.Errorf("event type %q: %w", ev.EventType, ErrEventNotAllowed)
	}
	res, err := s.db.NewInsert().
		Model(&processedEventRow{BaseModel: bun.BaseModel{}, CaseID: ev.CaseID, EventType: ev.EventType}).
		Ignore().
		Exec(ctx)
	if err != nil {
		s.metrics.IncEnqueueFailures()
		s.metrics.IncReceived(ev.EventType, "error")
		return EnqueueResult{}, fmt.Errorf("insert processed event: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		s.metrics.IncEnqueueFailures()
		s.metrics.IncReceived(ev.EventType, "error")
		return EnqueueResult{}, fmt.Errorf("processed event rows affected: %w", err)
	}
	if affected == 0 {
		s.metrics.IncDedupSkipped()
		s.metrics.IncReceived(ev.EventType, "duplicate")
		return EnqueueResult{Duplicate: true}, nil
	}
	// Crash window: the marker is committed before the enqueue, so a crash
	// in between leaves a marker without a worker event. The event is then
	// treated as processed and never replayed. Accepted MVP trade-off.
	if enqueueErr := s.workerSvc.Enqueue(ctx, ev); enqueueErr != nil {
		s.deleteMarker(ctx, ev)
		s.metrics.IncEnqueueFailures()
		s.metrics.IncReceived(ev.EventType, "error")
		return EnqueueResult{}, fmt.Errorf("enqueue event: %w", enqueueErr)
	}
	s.metrics.IncReceived(ev.EventType, "accepted")
	return EnqueueResult{}, nil
}

// deleteMarker removes the dedup marker after a full-queue handoff failure
// so the caller can return 500 and OmniDesk retries the event later.
func (s *Service) deleteMarker(ctx context.Context, ev worker.Event) {
	if _, err := s.db.NewDelete().
		Model(&processedEventRow{BaseModel: bun.BaseModel{}, CaseID: ev.CaseID, EventType: ev.EventType}).
		Where("case_id = ? AND event_type = ?", ev.CaseID, ev.EventType).
		Exec(ctx); err != nil {
		s.logger.Error("failed to delete processed marker after full-queue handoff",
			zap.Error(err),
			zap.Int64("case_id", ev.CaseID),
			zap.String("event_type", ev.EventType),
		)
	}
}
