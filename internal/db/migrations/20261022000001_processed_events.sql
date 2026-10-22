-- +goose Up
-- +goose StatementBegin
-- The dedup key must distinguish events inside one case: a case receives
-- many messages (comments) and every message of the same event type
-- produced the same (case_id, event_type) pair, so the second and later
-- messages were dropped as duplicates. subject_id is the normalized
-- identity of the concrete thing the event is about within the case.
--
-- The unique key (case_id, event_type, subject_id) stays inside the
-- InnoDB index length limit: 8 bytes (BIGINT) + 2 * 256 bytes
-- (VARCHAR(64) utf8mb4) = 520 bytes, well below the 3072-byte limit of
-- the DYNAMIC row format.
CREATE TABLE processed_events (
    id BIGINT NOT NULL AUTO_INCREMENT,
    case_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    subject_id VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_dedup (case_id, event_type, subject_id)
) ENGINE = InnoDB;
-- +goose StatementEnd
---
-- +goose Down
-- +goose StatementBegin
DROP TABLE processed_events;
-- +goose StatementEnd
