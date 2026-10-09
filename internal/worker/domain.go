package worker

import "context"

// Event is a webhook event carrying only identifiers plus the event type.
type Event struct {
	EventType string
	CaseID    int64
	// Payload is the raw webhook body JSON, stored verbatim.
	Payload []byte
}

// JobHandler executes a single event. Errors are logged by the worker and
// the event is dropped (fire-and-forget); there is no retry.
type JobHandler func(ctx context.Context, ev Event) error
