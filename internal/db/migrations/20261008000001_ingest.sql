-- +goose Up
-- +goose StatementBegin
CREATE TABLE processed_events (
    id BIGINT NOT NULL AUTO_INCREMENT,
    case_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_dedup (case_id, event_type)
) ENGINE = InnoDB;
-- +goose StatementEnd
---
-- +goose Down
-- +goose StatementBegin
DROP TABLE processed_events;
-- +goose StatementEnd