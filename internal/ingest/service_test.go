package ingest_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/worker"
	"go.uber.org/zap"
)

const testSecretHeader = "Authorization"

func testConfig() ingest.Config {
	return ingest.Config{
		Secret:        "test-secret",
		SecretHeader:  testSecretHeader,
		AllowedEvents: []string{ingest.EventTypeTicketMessageCreated, ingest.EventTypeTicketClosed},
	}
}

func newTestService(config ingest.Config) *ingest.Service {
	reg := prometheus.NewRegistry()
	workerSvc := worker.New(
		worker.Config{QueueSize: 4},
		worker.NewMetrics(reg),
		worker.NewStubHandler(zap.NewNop()),
		zap.NewNop(),
	)
	return ingest.New(config, ingest.NewRepository(nil), workerSvc, ingest.NewMetrics(reg), zap.NewNop())
}

func TestServiceSecretHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService(testConfig())
	if got := svc.SecretHeader(); got != testSecretHeader {
		t.Fatalf("SecretHeader() = %q, want %q", got, testSecretHeader)
	}
}

func TestServiceValidateSecret(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		config ingest.Config
		key    string
		want   bool
	}{
		{name: "matching secret", config: testConfig(), key: "test-secret", want: true},
		{name: "wrong secret", config: testConfig(), key: "wrong-secret", want: false},
		{name: "empty key", config: testConfig(), key: "", want: false},
		{
			name:   "unconfigured secret",
			config: ingest.Config{Secret: "", SecretHeader: testSecretHeader, AllowedEvents: nil},
			key:    "test-secret",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := newTestService(tc.config).ValidateSecret(tc.key); got != tc.want {
				t.Fatalf("ValidateSecret(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestConfigIsAllowed(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cases := []struct {
		name      string
		config    ingest.Config
		eventType string
		want      bool
	}{
		{name: "allowed literal", config: cfg, eventType: ingest.EventTypeTicketMessageCreated, want: true},
		{name: "second allowed literal", config: cfg, eventType: ingest.EventTypeTicketClosed, want: true},
		{name: "unknown literal rejected", config: cfg, eventType: "ticket.nonexistent", want: false},
		{name: "empty event type rejected", config: cfg, eventType: "", want: false},
		{
			name:      "empty allowlist rejects all",
			config:    ingest.Config{Secret: "s", SecretHeader: testSecretHeader, AllowedEvents: []string{}},
			eventType: ingest.EventTypeTicketMessageCreated,
			want:      false,
		},
		{
			name:      "nil allowlist rejects all",
			config:    ingest.Config{Secret: "s", SecretHeader: testSecretHeader, AllowedEvents: nil},
			eventType: ingest.EventTypeTicketClosed,
			want:      false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.config.IsAllowed(tc.eventType); got != tc.want {
				t.Fatalf("IsAllowed(%q) = %v, want %v", tc.eventType, got, tc.want)
			}
		})
	}
}
