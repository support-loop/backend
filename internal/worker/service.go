package worker

import (
	"context"

	"go.uber.org/zap"
)

// Service owns the in-process event queue and consumes it in a background
// goroutine. The queue is an internal implementation detail: callers enqueue
// through Enqueue and never see the channel.
type Service struct {
	queue   chan Event
	metrics *Metrics
	handler JobHandler
	logger  *zap.Logger
}

// New creates the worker service with an internal queue of cfg.QueueSize
// capacity.
func New(cfg Config, metrics *Metrics, handler JobHandler, logger *zap.Logger) *Service {
	return &Service{
		queue:   make(chan Event, cfg.QueueSize),
		metrics: metrics,
		handler: handler,
		logger:  logger,
	}
}

// Enqueue hands an event to the in-process queue without blocking. It
// returns ErrChannelFull when the queue is full; the caller decides whether
// to compensate (e.g. delete a dedup marker) and surface a 500.
func (s *Service) Enqueue(_ context.Context, ev Event) error {
	select {
	case s.queue <- ev:
		return nil
	default:
		return ErrChannelFull
	}
}

// Run consumes the queue until ctx is done. A single consumer goroutine is
// enough: the stub handler is trivial and drains at memory speed, and the
// bounded queue plus the webhook compensating delete cover backpressure.
func (s *Service) Run(ctx context.Context) error {
	s.updateQueueDepth()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-s.queue:
			s.process(ctx, ev)
		}
	}
}

func (s *Service) process(ctx context.Context, ev Event) {
	if handlerErr := s.handler(ctx, ev); handlerErr != nil {
		// Fire-and-forget: log the failure and drop the event. There is
		// no retry, requeue or failed state at this stage.
		s.logger.Error("event handler failed; dropping event",
			zap.Error(handlerErr),
			zap.String("event_type", ev.EventType),
			zap.Int64("case_id", ev.CaseID),
		)
	}
	s.metrics.ObserveJob(ev.EventType)
	s.updateQueueDepth()
}

func (s *Service) updateQueueDepth() {
	s.metrics.SetQueueDepth(float64(len(s.queue)))
}
