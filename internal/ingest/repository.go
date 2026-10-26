package ingest

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Repository stores the processed_events dedup markers. It is the only place
// in the ingest domain that talks to the database.
type Repository struct {
	db *bun.DB
}

// NewRepository creates the ingest repository.
func NewRepository(db *bun.DB) *Repository {
	return &Repository{db: db}
}

// InsertMarker inserts the dedup marker of the event and reports whether a new
// marker was created. A false return means the (case_id, event_type,
// subject_id) key already existed, that is the event is a duplicate.
func (r *Repository) InsertMarker(ctx context.Context, key DedupKey) (bool, error) {
	res, err := r.db.NewInsert().
		Model(&processedEventRow{
			BaseModel: bun.BaseModel{},
			CaseID:    key.CaseID,
			EventType: key.EventType,
			SubjectID: key.SubjectID,
		}).
		Ignore().
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("insert processed event: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("processed event rows affected: %w", err)
	}
	return affected > 0, nil
}

// DeleteMarker removes the dedup marker matching the whole key. It matches all
// three key columns on purpose: a case holds many events of the same type, so
// dropping only one column would leave a false duplicate behind or delete the
// markers of other subjects.
func (r *Repository) DeleteMarker(ctx context.Context, key DedupKey) error {
	_, err := r.db.NewDelete().
		Model(&processedEventRow{
			BaseModel: bun.BaseModel{},
			CaseID:    key.CaseID,
			EventType: key.EventType,
			SubjectID: key.SubjectID,
		}).
		Where("case_id = ? AND event_type = ? AND subject_id = ?", key.CaseID, key.EventType, key.SubjectID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete processed event: %w", err)
	}
	return nil
}
