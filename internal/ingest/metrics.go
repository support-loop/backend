package ingest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	metricsNamespace = "assistant"
	metricsSubsystem = "webhook"
)

// Metrics collects ingest webhook Prometheus metrics.
type Metrics struct {
	receivedTotal   *prometheus.CounterVec
	unknownEvent    prometheus.Counter
	dedupSkipped    prometheus.Counter
	enqueueFailures prometheus.Counter
}

// NewMetrics creates the ingest metrics on the given registerer.
// The registerer is passed in so tests can use isolated registries.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		receivedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "received_total",
			Help:      "Total number of webhook events received, by event type and status",
		}, []string{"event", "status"}),
		unknownEvent: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "unknown_event_total",
			Help:      "Total number of webhook events with an event type outside the allowlist",
		}),
		dedupSkipped: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "dedup_skipped_total",
			Help:      "Total number of duplicate webhook events skipped",
		}),
		enqueueFailures: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "enqueue_failures_total",
			Help:      "Total number of webhook events that failed to enqueue",
		}),
	}
}

// IncReceived records a webhook event with its outcome status.
func (m *Metrics) IncReceived(eventType, status string) {
	m.receivedTotal.WithLabelValues(eventType, status).Inc()
}

// IncUnknownEvent increments the unknown event type counter.
func (m *Metrics) IncUnknownEvent() {
	m.unknownEvent.Inc()
}

// IncDedupSkipped increments the dedup skip counter.
func (m *Metrics) IncDedupSkipped() {
	m.dedupSkipped.Inc()
}

// IncEnqueueFailures increments the enqueue failure counter.
func (m *Metrics) IncEnqueueFailures() {
	m.enqueueFailures.Inc()
}
