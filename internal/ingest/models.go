package ingest

import "github.com/uptrace/bun"

// processedEventRow is the processed_events dedup marker row. created_at is
// insert-time-only and defaults to CURRENT_TIMESTAMP(3) in the migration, so
// the row never sets it in code. subject_id is part of the dedup key: it
// tells the events of one case apart (a case receives many messages, every
// one of them a separate event of the same type).
type processedEventRow struct {
	bun.BaseModel `bun:"table:processed_events"`

	CaseID    int64  `bun:"case_id,notnull,nullzero"`
	EventType string `bun:"event_type,notnull,nullzero"`
	SubjectID string `bun:"subject_id,notnull"`
}
