package webhook_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/worker"
	"go.uber.org/zap"
)

// webhookMessagePayload builds the body of our own rule template for the
// message rule: the subject_id field is injected by the template, exactly
// like event_type.
func webhookMessagePayload(caseID int64, subjectID string) string {
	return `{"event_type":"` + ingest.EventTypeTicketMessageCreated + `","case_id":"` +
		strconv.FormatInt(caseID, 10) + `","subject_id":"` + subjectID + `"}`
}

// waitEvent returns the next processed event and fails when none arrives.
func waitEvent(t *testing.T, events chan worker.Event) worker.Event {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not process the event within timeout")
		return worker.Event{}
	}
}

// assertNoEvent proves that no further event is processed within the grace
// period: the deduplicated delivery must not reach the worker.
func assertNoEvent(t *testing.T, events chan worker.Event) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("unexpected extra event processed: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestWebhookTwoMessagesSameCaseBothProcessed is the regression test of the
// dedup bug: a case receives many messages, every one of them an event of the
// same type. The dedup key (case_id, event_type) dropped the second and later
// messages, so Flow 1 saw only the first message of the ticket. With the
// subject in the key, both messages are processed.
func TestWebhookTwoMessagesSameCaseBothProcessed(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	events := make(chan worker.Event, 4)
	app, reg, workerSvc := newIntegrationApp(t, db, 8, func(_ context.Context, ev worker.Event) error {
		events <- ev
		return nil
	})
	caseID := time.Now().UnixNano()

	for _, subjectID := range []string{"message-1", "message-2"} {
		resp := doPost(t, app, webhookMessagePayload(caseID, subjectID), testSecret)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}
	if got := countMarkers(t, db, caseID); got != 2 {
		t.Fatalf("processed markers = %d, want 2", got)
	}

	stop := startWorker(t, workerSvc)
	defer stop()
	seen := make(map[string]bool, 2)
	for range 2 {
		seen[waitEvent(t, events).SubjectID] = true
	}
	assertNoEvent(t, events)
	if len(seen) != 2 {
		t.Fatalf("processed subjects = %v, want 2 distinct subjects", seen)
	}
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketMessageCreated}, 2)
}

// TestWebhookSameSubjectDeduped proves the repeated delivery of one message
// is deduplicated: one marker, one worker execution.
func TestWebhookSameSubjectDeduped(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	events := make(chan worker.Event, 4)
	app, reg, workerSvc := newIntegrationApp(t, db, 8, func(_ context.Context, ev worker.Event) error {
		events <- ev
		return nil
	})
	caseID := time.Now().UnixNano()
	payload := webhookMessagePayload(caseID, "message-1")

	for range 2 {
		resp := doPost(t, app, payload, testSecret)
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
	defer stop()
	waitEvent(t, events)
	assertNoEvent(t, events)
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketMessageCreated}, 1)
}

// TestWebhookMixedSubjectsDedup proves that two messages of one case are told
// apart while each of them is deduplicated on its own: every subject is
// delivered twice, the third delivery of the first subject is deduplicated
// too, and exactly two events reach the worker.
func TestWebhookMixedSubjectsDedup(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	events := make(chan worker.Event, 4)
	app, reg, workerSvc := newIntegrationApp(t, db, 8, func(_ context.Context, ev worker.Event) error {
		events <- ev
		return nil
	})
	caseID := time.Now().UnixNano()

	for _, subjectID := range []string{"message-1", "message-2", "message-1", "message-2", "message-1"} {
		resp := doPost(t, app, webhookMessagePayload(caseID, subjectID), testSecret)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}
	if got := countMarkers(t, db, caseID); got != 2 {
		t.Fatalf("processed markers = %d, want 2", got)
	}
	if got := counterValue(t, reg, "assistant_webhook_dedup_skipped_total", nil); got != 3 {
		t.Fatalf("dedup skipped = %v, want 3", got)
	}

	stop := startWorker(t, workerSvc)
	defer stop()
	seen := make(map[string]bool, 2)
	for range 2 {
		seen[waitEvent(t, events).SubjectID] = true
	}
	assertNoEvent(t, events)
	if len(seen) != 2 {
		t.Fatalf("processed subjects = %v, want 2 distinct subjects", seen)
	}
	waitForCounter(t, reg, map[string]string{"event": ingest.EventTypeTicketMessageCreated}, 2)
}

// TestWebhookSubjectIDFromTemplate proves the subject_id injected by our rule
// template reaches the dedup marker; the native last_message_id of the same
// body is not used.
func TestWebhookSubjectIDFromTemplate(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, _, _ := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	caseID := time.Now().UnixNano()
	body := `{"event_type":"ticket.message.created","case_id":"` + strconv.FormatInt(caseID, 10) +
		`","subject_id":"injected-subject","last_message_id":"638929422"}`

	resp := doPost(t, app, body, testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := countMarkersBySubject(t, db, caseID, "injected-subject"); got != 1 {
		t.Fatalf("markers of the injected subject = %d, want 1", got)
	}
	if got := countMarkersBySubject(t, db, caseID, "638929422"); got != 0 {
		t.Fatalf("markers of the native last message id = %d, want 0", got)
	}
}

// TestWebhookSubjectIDFallsBackToLastMessageID proves that a body of the
// standard OmniDesk template (without our injected subject_id) still works:
// the native last_message_id becomes the subject.
func TestWebhookSubjectIDFallsBackToLastMessageID(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, _, _ := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	caseID := time.Now().UnixNano()
	body := `{"event_type":"ticket.message.created","case_id":"` + strconv.FormatInt(caseID, 10) +
		`","last_message_id":"638929422"}`

	resp := doPost(t, app, body, testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := countMarkersBySubject(t, db, caseID, "638929422"); got != 1 {
		t.Fatalf("markers of the native last message id = %d, want 1", got)
	}
}

// TestWebhookWithoutSubjectDedupsAtCaseLevel proves that a body without any
// subject (a case-level event such as ticket.closed) still works: the
// subject is empty and the event deduplicates at case level.
func TestWebhookWithoutSubjectDedupsAtCaseLevel(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	app, reg, _ := newIntegrationApp(t, db, 8, worker.NewStubHandler(zap.NewNop()))
	caseID := time.Now().UnixNano()
	body := `{"event_type":"ticket.closed","case_id":"` + strconv.FormatInt(caseID, 10) + `"}`

	for range 2 {
		resp := doPost(t, app, body, testSecret)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}
	if got := countMarkers(t, db, caseID); got != 1 {
		t.Fatalf("processed markers = %d, want 1", got)
	}
	if got := countMarkersBySubject(t, db, caseID, ""); got != 1 {
		t.Fatalf("markers of the empty subject = %d, want 1", got)
	}
	if got := counterValue(t, reg, "assistant_webhook_dedup_skipped_total", nil); got != 1 {
		t.Fatalf("dedup skipped = %v, want 1", got)
	}
}
