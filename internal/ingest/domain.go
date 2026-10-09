package ingest

import "github.com/uptrace/bun"

// EventType literals authored by us in the webhook body template.
const (
	EventTypeTicketMessageCreated = "ticket.message.created"
	EventTypeTicketClosed         = "ticket.closed"
)

// EnqueueResult reports whether the event was already known.
type EnqueueResult struct {
	Duplicate bool
}

// processedEventRow is the processed_events dedup marker row. created_at is
// insert-time-only and defaults to CURRENT_TIMESTAMP(3) in the migration, so
// the row never sets it in code.
type processedEventRow struct {
	bun.BaseModel `bun:"table:processed_events"`

	CaseID    int64  `bun:"case_id,notnull,nullzero"`
	EventType string `bun:"event_type,notnull,nullzero"`
}
