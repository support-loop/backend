package worker

import "context"

// Event is a webhook event carrying only identifiers plus the event type.
type Event struct {
	EventType string
	CaseID    int64
	// SubjectID is the normalized identity of the concrete thing the event
	// is about within the case (for OmniDesk: the id of the message the
	// event is about). It is part of the dedup key, so two events of the
	// same type inside one case are told apart by it. It is empty for
	// case-level events (for example a case being closed).
	SubjectID string
	// Payload is the raw webhook body JSON, stored verbatim.
	Payload []byte
}

// JobHandler executes a single event. Errors are logged by the worker and
// the event is dropped (fire-and-forget); there is no retry.
type JobHandler func(ctx context.Context, ev Event) error
