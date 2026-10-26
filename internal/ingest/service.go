package ingest

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/support-loop/backend/internal/worker"
	"go.uber.org/zap"
)

// markerDeleteTimeout bounds the detached dedup-marker cleanup after a
// failed enqueue.
const markerDeleteTimeout = 5 * time.Second

// Service validates webhook events, deduplicates them and hands them to the
// worker via its public Enqueue method.
type Service struct {
	config          Config
	processedEvents *Repository
	workerSvc       *worker.Service
	metrics         *Metrics
	logger          *zap.Logger
}

// New creates the ingest service.
func New(
	config Config,
	processedEvents *Repository,
	workerSvc *worker.Service,
	metrics *Metrics,
	logger *zap.Logger,
) *Service {
	return &Service{
		config:          config,
		processedEvents: processedEvents,
		workerSvc:       workerSvc,
		metrics:         metrics,
		logger:          logger,
	}
}

// SecretHeader returns the header name carrying the shared secret.
func (s *Service) SecretHeader() string {
	return s.config.SecretHeader
}

// ValidateSecret reports whether key matches the configured shared secret,
// compared in constant time.
func (s *Service) ValidateSecret(key string) bool {
	return subtle.ConstantTimeCompare([]byte(key), []byte(s.config.Secret)) == 1
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
	key := DedupKey{
		CaseID:    ev.CaseID,
		EventType: ev.EventType,
		SubjectID: ev.SubjectID,
	}
	inserted, err := s.processedEvents.InsertMarker(ctx, key)
	if err != nil {
		s.metrics.IncEnqueueFailures()
		s.metrics.IncReceived(ev.EventType, "error")
		return EnqueueResult{}, err
	}
	if !inserted {
		s.metrics.IncDedupSkipped()
		s.metrics.IncReceived(ev.EventType, "duplicate")
		return EnqueueResult{Duplicate: true}, nil
	}
	// Crash window: the marker is committed before the enqueue, so a crash
	// in between leaves a marker without a worker event. The event is then
	// treated as processed and never replayed. Accepted MVP trade-off.
	if enqueueErr := s.workerSvc.Enqueue(ctx, ev); enqueueErr != nil {
		s.deleteMarker(ctx, key)
		s.metrics.IncEnqueueFailures()
		s.metrics.IncReceived(ev.EventType, "error")
		return EnqueueResult{}, fmt.Errorf("enqueue event: %w", enqueueErr)
	}
	s.metrics.IncReceived(ev.EventType, "accepted")
	return EnqueueResult{}, nil
}

// deleteMarker removes the dedup marker after a full-queue handoff failure
// so the caller can return 500 and OmniDesk retries the event later. The
// cleanup runs under a detached context so a request cancellation cannot
// abort it and leave a permanent false-duplicate marker.
func (s *Service) deleteMarker(ctx context.Context, key DedupKey) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), markerDeleteTimeout)
	defer cancel()
	if err := s.processedEvents.DeleteMarker(ctx, key); err != nil {
		s.logger.Error("failed to delete processed marker after full-queue handoff",
			zap.Error(err),
			zap.Int64("case_id", key.CaseID),
			zap.String("event_type", key.EventType),
			zap.String("subject_id", key.SubjectID),
		)
	}
}
