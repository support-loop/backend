package webhook_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/fiberfx/validation"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/server/handlers/webhook"
	"github.com/support-loop/backend/internal/worker"
	"go.uber.org/zap"
)

const (
	testSecret       = "test-secret"
	testSecretHeader = "Authorization"
)

func testIngestConfig() ingest.Config {
	return ingest.Config{
		Secret:        testSecret,
		SecretHeader:  testSecretHeader,
		AllowedEvents: []string{ingest.EventTypeTicketMessageCreated, ingest.EventTypeTicketClosed},
	}
}

// newTestApp wires the handler against a nil-DB service. The cases covered
// here (auth, parse, validation, allowlist) all return before the DB or the
// worker queue is touched, so both stay unused. The webhook group registers
// its own error handler; validation.Middleware stays at the API group.
func newTestApp(cfg ingest.Config) (*fiber.App, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	metrics := ingest.NewMetrics(reg)
	workerSvc := worker.New(
		worker.Config{QueueSize: 4},
		worker.NewMetrics(prometheus.NewRegistry()),
		worker.NewStubHandler(zap.NewNop()),
		zap.NewNop(),
	)
	svc := ingest.New(cfg, nil, workerSvc, metrics, zap.NewNop())
	// The fiberfx JSON error handler mirrors the server module's wiring so
	// the mapped responses are formatted exactly as in production.
	app := fiber.New(fiber.Config{
		ErrorHandler: fiberfx.NewJSONErrorHandler(zap.NewNop()),
	})
	v1 := app.Group("/api/v1")
	v1.Use(validation.Middleware)
	h := webhook.New(svc, cfg, validator.New())
	h.Register(v1)
	return app, reg
}

func doPost(t *testing.T, app *fiber.App, body, secret string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/omnidesk", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(testSecretHeader, secret)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func responseBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}

func counterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric.GetLabel(), labels) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labelsMatch(got []*dto.LabelPair, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, label := range got {
		if want[label.GetName()] != label.GetValue() {
			return false
		}
	}
	return true
}

func validBody() string {
	return `{"event_type":"ticket.message.created","case_id":"123456789","last_message_id":"98765"}`
}

func TestWebhookMissingSecretForbidden(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(testIngestConfig())
	resp := doPost(t, app, validBody(), "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWebhookWrongSecretForbidden(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(testIngestConfig())
	resp := doPost(t, app, validBody(), "wrong-secret")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWebhookUnconfiguredSecretForbidden(t *testing.T) {
	t.Parallel()
	cfg := testIngestConfig()
	cfg.Secret = ""
	app, _ := newTestApp(cfg)
	resp := doPost(t, app, validBody(), "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWebhookBadBodyBadRequest(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(testIngestConfig())
	resp := doPost(t, app, `{not-json`, testSecret)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestWebhookInvalidBodyBadRequest(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(testIngestConfig())
	resp := doPost(t, app, `{"event_type":"ticket.message.created"}`, testSecret)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestWebhookNonNumericCaseIDBadRequest(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(testIngestConfig())
	resp := doPost(t, app, `{"event_type":"ticket.message.created","case_id":"abc"}`, testSecret)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestWebhookNonStringCaseIDBadRequest proves a non-string case_id token
// yields the [json.UnmarshalTypeError] mapping to 400.
func TestWebhookNonStringCaseIDBadRequest(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(testIngestConfig())
	resp := doPost(t, app, `{"event_type":"ticket.message.created","case_id":123}`, testSecret)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestWebhookUnknownEventTypeIgnored(t *testing.T) {
	t.Parallel()
	app, reg := newTestApp(testIngestConfig())
	body := `{"event_type":"ticket.unknown","case_id":"123"}`
	resp := doPost(t, app, body, testSecret)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// SendStatus(200) answers with the "OK" body; OmniDesk stops retrying.
	if got := responseBody(t, resp); got != "OK" {
		t.Fatalf("body = %q, want OK", got)
	}
	if got := counterValue(t, reg, "assistant_unknown_event_total", nil); got != 1 {
		t.Fatalf("unknown events = %v, want 1", got)
	}
	if got := counterValue(
		t,
		reg,
		"assistant_webhook_received_total",
		map[string]string{"event": "ticket.unknown", "status": "ignored"},
	); got != 1 {
		t.Fatalf("received ignored = %v, want 1", got)
	}
}
