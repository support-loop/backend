package worker

import "errors"

// ErrChannelFull is returned by Enqueue when the in-process queue is full.
// Callers compensate (e.g. delete a dedup marker) and surface a 500 so the
// upstream system retries the event later.
var ErrChannelFull = errors.New("worker queue full")
