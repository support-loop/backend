package ingest_test

import (
	"testing"

	"github.com/support-loop/backend/internal/ingest"
)

const testSecretHeader = "Authorization"

func testConfig() ingest.Config {
	return ingest.Config{
		Secret:        "test-secret",
		SecretHeader:  testSecretHeader,
		AllowedEvents: []string{ingest.EventTypeTicketMessageCreated, ingest.EventTypeTicketClosed},
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
