package webhook_test

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/fiberfx/validation"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/server/handlers/webhook"
	"github.com/support-loop/backend/internal/worker"
	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

const processedEventsDDL = `
	CREATE TABLE processed_events (
		id BIGINT NOT NULL AUTO_INCREMENT,
		case_id BIGINT NOT NULL,
		event_type VARCHAR(64) NOT NULL,
		created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		PRIMARY KEY (id),
		UNIQUE KEY uq_dedup (case_id, event_type)
	) ENGINE = InnoDB`

func webhookPayload(eventType string, caseID int64) string {
	return `{"event_type":"` + eventType + `","case_id":"` + strconv.FormatInt(
		caseID,
		10,
	) + `","last_message_id":"98765"}`
}

// newIntegrationApp wires the webhook handler over a real DB and a real
// worker service whose queue has the given capacity. Ingest and worker
// metrics share one registry so tests can wait on worker counters.
func newIntegrationApp(
	t *testing.T,
	db *bun.DB,
	capacity int,
	handler worker.JobHandler,
) (*fiber.App, *prometheus.Registry, *worker.Service) {
	t.Helper()
	logger := zap.NewNop()
	reg := prometheus.NewRegistry()
	metrics := ingest.NewMetrics(reg)
	workerSvc := worker.New(worker.Config{QueueSize: capacity}, worker.NewMetrics(reg), handler, logger)
	cfg := ingest.Config{
		Secret:        testSecret,
		SecretHeader:  testSecretHeader,
		AllowedEvents: []string{ingest.EventTypeTicketMessageCreated, ingest.EventTypeTicketClosed},
	}
	svc := ingest.New(cfg, db, workerSvc, metrics, logger)
	// The fiberfx JSON error handler mirrors the server module's wiring so
	// the mapped responses are formatted exactly as in production.
	app := fiber.New(fiber.Config{
		ErrorHandler: fiberfx.NewJSONErrorHandler(logger),
	})
	v1 := app.Group("/api/v1")
	v1.Use(validation.Middleware)
	h := webhook.New(svc, cfg, validator.New())
	h.Register(v1)
	return app, reg, workerSvc
}

// startWorker runs the worker service and returns its stop closure.
func startWorker(t *testing.T, svc *worker.Service) func() {
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

func waitForCounter(t *testing.T, reg *prometheus.Registry, labels map[string]string, want float64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if counterValue(t, reg, "assistant_jobs_total", labels) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("counter assistant_jobs_total did not reach %v within timeout", want)
}

// TestWebhookAccepted proves a single valid submission yields exactly one
// dedup marker, one accepted event and exactly one worker execution.
func TestWebhookAccepted(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, reg, workerSvc := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	caseID := time.Now().UnixNano()

	resp := doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, caseID), testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := counterValue(
		t,
		reg,
		"assistant_webhook_received_total",
		map[string]string{"event": ingest.EventTypeTicketMessageCreated, "status": "accepted"},
	); got != 1 {
		t.Fatalf("received accepted = %v, want 1", got)
	}
	if got := countMarkers(t, db, caseID); got != 1 {
		t.Fatalf("processed markers = %d, want 1", got)
	}

	stop := startWorker(t, workerSvc)
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketMessageCreated}, 1)
	stop()
}

// TestWebhookDuplicateAccepted proves the double-submit path: the same
// (case_id, event_type) twice yields exactly one dedup marker and exactly
// one worker execution (jobs_total = 1).
func TestWebhookDuplicateAccepted(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, reg, workerSvc := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	caseID := time.Now().UnixNano()

	for range 2 {
		resp := doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, caseID), testSecret)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}
	if got := countMarkers(t, db, caseID); got != 1 {
		t.Fatalf("processed markers = %d, want 1", got)
	}
	if got := counterValue(t, reg, "assistant_webhook_dedup_skipped_total", nil); got != 1 {
		t.Fatalf("dedup skipped = %v, want 1", got)
	}

	stop := startWorker(t, workerSvc)
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketMessageCreated}, 1)
	stop()
}

// TestWebhookChannelFullCompensates proves the full-queue handoff path: the
// marker is deleted (compensating action), the handler returns 500, and a
// retry after the queue drains succeeds. The queue is filled through the
// webhook itself: the first event blocks the worker handler, the next two
// fill the buffer.
func TestWebhookChannelFullCompensates(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	blocked := make(chan struct{}, 1)
	release := make(chan struct{})
	blocking := func(_ context.Context, _ worker.Event) error {
		// Signal only the first blocking event; the buffer keeps the
		// signal until the test receives it and later sends are skipped.
		select {
		case blocked <- struct{}{}:
		default:
		}
		<-release
		return nil
	}
	app, reg, workerSvc := newIntegrationApp(t, db, 2, blocking)
	base := time.Now().UnixNano()
	blockedID := base + 3

	stop := startWorker(t, workerSvc)
	defer stop()

	// First event is consumed and blocks the worker handler.
	resp := doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, base), testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("blocking event status = %d, want 200", resp.StatusCode)
	}
	select {
	case <-blocked:
	case <-time.After(10 * time.Second):
		t.Fatalf("worker did not block in handler")
	}

	// Two more events fill the buffer; the next one must fail.
	for i := 1; i <= 2; i++ {
		r := doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, base+int64(i)), testSecret)
		if r.StatusCode != http.StatusOK {
			t.Fatalf("filling event %d status = %d, want 200", i, r.StatusCode)
		}
	}
	resp = doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, blockedID), testSecret)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("blocked event status = %d, want 500", resp.StatusCode)
	}
	if got := counterValue(t, reg, "assistant_webhook_enqueue_failures_total", nil); got != 1 {
		t.Fatalf("enqueue failures = %v, want 1", got)
	}
	if got := countMarkers(t, db, blockedID); got != 0 {
		t.Fatalf("processed markers after compensation = %d, want 0", got)
	}

	// Drain the queue; the retry then succeeds.
	close(release)
	resp = doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, blockedID), testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", resp.StatusCode)
	}
	if got := countMarkers(t, db, blockedID); got != 1 {
		t.Fatalf("processed markers after retry = %d, want 1", got)
	}
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketMessageCreated}, 4)
}

// TestWebhookInsertFailure proves an INSERT IGNORE failure yields 500 with
// nothing to compensate, and a redelivery after the failure is fixed
// succeeds. The 500 body is fiberfx's sanitized JSON error: the internal
// database error never leaks.
func TestWebhookInsertFailure(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, reg, _ := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	ctx := context.Background()
	caseID := time.Now().UnixNano()

	// Drop the dedup table to force the INSERT IGNORE to fail.
	if _, err := db.NewRaw(`DROP TABLE processed_events`).Exec(ctx); err != nil {
		t.Fatalf("drop processed_events: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.NewRaw(processedEventsDDL).Exec(context.Background())
	})

	resp := doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, caseID), testSecret)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "Internal Server Error") {
		t.Fatalf("body = %q, want sanitized 500 message", got)
	}
	if strings.Contains(got, "processed_events") {
		t.Fatalf("body = %q, leaks internal error", got)
	}
	if got := counterValue(t, reg, "assistant_webhook_enqueue_failures_total", nil); got != 1 {
		t.Fatalf("enqueue failures = %v, want 1", got)
	}

	// Recreate the table: no marker was written for the failed attempt.
	if _, rerr := db.NewRaw(processedEventsDDL).Exec(ctx); rerr != nil {
		t.Fatalf("recreate processed_events: %v", rerr)
	}
	if got := countMarkers(t, db, caseID); got != 0 {
		t.Fatalf("processed markers after failed insert = %d, want 0", got)
	}

	resp = doPost(t, app, webhookPayload(ingest.EventTypeTicketMessageCreated, caseID), testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redelivery status = %d, want 200", resp.StatusCode)
	}
	if got := countMarkers(t, db, caseID); got != 1 {
		t.Fatalf("processed markers after redelivery = %d, want 1", got)
	}
}

// TestWebhookFullCycleIntegration runs the whole slice over HTTP: enqueue,
// worker consumption, stub execution, dedup marker retained.
func TestWebhookFullCycleIntegration(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, reg, workerSvc := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	caseID := time.Now().UnixNano()

	resp := doPost(t, app, webhookPayload(ingest.EventTypeTicketClosed, caseID), testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	stop := startWorker(t, workerSvc)
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketClosed}, 1)
	stop()
	if got := countMarkers(t, db, caseID); got != 1 {
		t.Fatalf("processed markers = %d, want 1", got)
	}
}
