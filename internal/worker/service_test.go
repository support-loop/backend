package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/support-loop/backend/internal/worker"
	"go.uber.org/zap"
)

func newTestService(t *testing.T, handler worker.JobHandler, capacity int) *worker.Service {
	t.Helper()
	return worker.New(
		worker.Config{QueueSize: capacity},
		worker.NewMetrics(prometheus.NewRegistry()),
		handler,
		zap.NewNop(),
	)
}

// runWorker starts the worker and returns a stop closure that cancels the
// run context and waits for Run to return without error.
func runWorker(t *testing.T, svc *worker.Service) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	return func() {
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("worker run failed: %v", err)
		}
	}
}

// waitReceived blocks until id arrives on the results channel or the
// deadline passes. The channel is the synchronization point, so the race
// detector sees no unsynchronized shared memory.
func waitReceived(t *testing.T, results chan int64, id int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case got := <-results:
			if got == id {
				return
			}
		default:
			if !time.Now().Before(deadline) {
				t.Fatalf("event %d not processed within timeout", id)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestWorkerProcessesReceivedEvents proves the worker consumes the queue and
// processes every event exactly once.
func TestWorkerProcessesReceivedEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	results := make(chan int64, 4)
	svc := newTestService(t, func(_ context.Context, ev worker.Event) error {
		results <- ev.CaseID
		return nil
	}, 4)
	stop := runWorker(t, svc)

	for i := range 3 {
		if err := svc.Enqueue(
			ctx,
			worker.Event{EventType: "ticket.message.created", CaseID: int64(i), Payload: []byte(`{}`)},
		); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	for i := range 3 {
		waitReceived(t, results, int64(i))
	}
	stop()
}

// TestWorkerLogsHandlerErrorsAndContinues proves a failing handler does not
// crash the worker: the error is logged, the event is dropped, and
// subsequent events still get processed.
func TestWorkerLogsHandlerErrorsAndContinues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	results := make(chan int64, 4)
	failing := func(_ context.Context, ev worker.Event) error {
		if ev.CaseID == 1 {
			return errors.New("injected failure")
		}
		results <- ev.CaseID
		return nil
	}
	svc := newTestService(t, failing, 4)
	stop := runWorker(t, svc)

	if err := svc.Enqueue(
		ctx,
		worker.Event{EventType: "ticket.message.created", CaseID: 1, Payload: []byte(`{}`)},
	); err != nil {
		t.Fatalf("enqueue 1: %v", err)
	}
	if err := svc.Enqueue(ctx, worker.Event{EventType: "ticket.closed", CaseID: 2, Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}
	waitReceived(t, results, 2)
	stop()
}

// TestWorkerEnqueueQueueFull proves Enqueue reports ErrChannelFull when the
// queue is full and accepts events while capacity is available.
func TestWorkerEnqueueQueueFull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestService(t, nil, 1)

	if err := svc.Enqueue(ctx, worker.Event{EventType: "ticket.closed", CaseID: 1, Payload: nil}); err != nil {
		t.Fatalf("enqueue into empty queue: %v", err)
	}
	if err := svc.Enqueue(
		ctx,
		worker.Event{EventType: "ticket.closed", CaseID: 2, Payload: nil},
	); !errors.Is(
		err,
		worker.ErrChannelFull,
	) {
		t.Fatalf("enqueue into full queue = %v, want ErrChannelFull", err)
	}
}
