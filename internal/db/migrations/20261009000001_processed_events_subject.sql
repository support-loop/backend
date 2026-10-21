-- +goose Up
-- +goose StatementBegin
-- The dedup key must distinguish events inside one case: a case receives
-- many messages (comments) and every message of the same event type
-- produced the same (case_id, event_type) pair, so the second and later
-- messages were dropped as duplicates. subject_id is the normalized
-- identity of the concrete thing the event is about within the case.
--
-- The new unique key (case_id, event_type, subject_id) stays inside the
-- InnoDB index length limit: 8 bytes (BIGINT) + 2 * 256 bytes
-- (VARCHAR(64) utf8mb4) = 520 bytes, well below the 3072-byte limit of
-- the DYNAMIC row format.
ALTER TABLE processed_events
ADD COLUMN subject_id VARCHAR(64) NOT NULL DEFAULT ''
AFTER event_type,
    DROP INDEX uq_dedup,
    ADD UNIQUE KEY uq_dedup (case_id, event_type, subject_id);
-- +goose StatementEnd
---
-- +goose Down
-- +goose StatementBegin
ALTER TABLE processed_events DROP INDEX uq_dedup,
    ADD UNIQUE KEY uq_dedup (case_id, event_type),
    DROP COLUMN subject_id;
-- +goose StatementEnd