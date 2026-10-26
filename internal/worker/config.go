// Package worker implements the in-process background processing service.
package worker

// Config holds the worker configuration.
type Config struct {
	// QueueSize is the capacity of the in-process event queue. The name is
	// implementation-neutral: it survives a future swap from a channel to a
	// real queue.
	QueueSize int
}
