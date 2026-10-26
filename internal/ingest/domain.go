package ingest

// EventType literals authored by us in the webhook body template.
const (
	EventTypeTicketMessageCreated = "ticket.message.created"
	EventTypeTicketClosed         = "ticket.closed"
)

// EnqueueResult reports whether the event was already known.
type EnqueueResult struct {
	Duplicate bool
}

// DedupKey identifies a webhook event for deduplication. It is the domain view
// of the processed_events key; the storage model lives in models.go and is
// never seen by the service.
type DedupKey struct {
	CaseID    int64
	EventType string
	// SubjectID is the normalized identity of the concrete thing the event is
	// about within the case. It is empty for case-level events.
	SubjectID string
}
