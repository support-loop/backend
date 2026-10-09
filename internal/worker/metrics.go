package worker

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const metricsNamespace = "assistant"

// Metrics collects worker Prometheus metrics.
type Metrics struct {
	queueDepth prometheus.Gauge
	jobsTotal  *prometheus.CounterVec
}

// NewMetrics creates the worker metrics on the given registerer.
// The registerer is passed in so tests can use isolated registries.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		queueDepth: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "queue_depth",
			Help:      "Current number of events buffered in the worker channel",
		}),
		jobsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "jobs_total",
			Help:      "Total number of events processed by the worker, by event type",
		}, []string{"event"}),
	}
}

// SetQueueDepth sets the buffered channel length gauge.
func (m *Metrics) SetQueueDepth(depth float64) {
	m.queueDepth.Set(depth)
}

// ObserveJob records a processed event.
func (m *Metrics) ObserveJob(eventType string) {
	m.jobsTotal.WithLabelValues(eventType).Inc()
}
