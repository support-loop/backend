package ingest

import "errors"

var (
	// ErrEventNotAllowed is returned when an event type is not on the allowlist.
	ErrEventNotAllowed = errors.New("event type not allowed")
)
