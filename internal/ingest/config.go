// Package ingest implements the webhook ingest domain: shared-secret auth,
// event_type allowlist, and dedup via the processed_events marker.
package ingest

import "github.com/samber/lo"

// Config holds the webhook ingest configuration.
type Config struct {
	// Secret is the shared secret expected in the configured header.
	// It is supplied via environment and must never be defaulted.
	Secret string
	// SecretHeader is the name of the header carrying the shared secret.
	// The name is our side of the webhook contract.
	SecretHeader string
	// AllowedEvents is the allowlist of event_type literals we author.
	AllowedEvents []string
}

// IsAllowed reports whether the event type is on the allowlist.
func (c Config) IsAllowed(eventType string) bool {
	return lo.Contains(c.AllowedEvents, eventType)
}
