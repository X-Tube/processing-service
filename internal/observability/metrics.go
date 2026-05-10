package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var SQSMessagesReceived = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sqs_messages_received_total",
		Help: "Total number of SQS messages received",
	},
	[]string{"queue"},
)

var SQSMessagesProcessed = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sqs_messages_processed_total",
		Help: "Total number of SQS messages processed",
	},
	[]string{"queue", "status"},
)

var SQSMessagesDeleted = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sqs_messages_deleted_total",
		Help: "Total number of SQS messages deleted",
	},
	[]string{"queue"},
)

var SQSProcessingDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "sqs_processing_duration_seconds",
		Help:    "SQS message processing duration in seconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"queue"},
)

func ObserveDuration(metric *prometheus.HistogramVec, queue string, startedAt time.Time) {
	metric.WithLabelValues(queue).Observe(time.Since(startedAt).Seconds())
}
