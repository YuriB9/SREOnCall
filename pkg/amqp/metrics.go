package amqp

import "github.com/prometheus/client_golang/prometheus"

// Bus golden signals, instrumented once here so every consumer and publisher is
// covered without per-service copy-paste. Labelled by queue/exchange, which
// already identify the pipeline stage at low, bounded cardinality.
//
// Useful PromQL:
//   - requeue rate (poison/transient failures):
//     sum(rate(amqp_messages_processed_total{result="requeue"}[5m])) by (queue)
//   - processing p99:
//     histogram_quantile(0.99, sum(rate(amqp_message_processing_seconds_bucket[5m])) by (le,queue))
//   - publish error ratio:
//     sum(rate(amqp_publish_total{result="error"}[5m])) by (exchange)
//     / sum(rate(amqp_publish_total[5m])) by (exchange)
//   - silent pipeline (no acks while messages should flow) — alert when:
//     sum(rate(amqp_messages_processed_total{result="ack"}[5m])) by (queue) == 0
var (
	messagesProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "amqp_messages_processed_total",
		Help: "AMQP deliveries processed, by queue and outcome (ack|requeue|drop)",
	}, []string{"queue", "result"})

	messageProcessingSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "amqp_message_processing_seconds",
		Help:    "Time spent handling a single AMQP delivery",
		Buckets: prometheus.DefBuckets,
	}, []string{"queue"})

	publishTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "amqp_publish_total",
		Help: "AMQP publishes attempted, by exchange and outcome (ok|error)",
	}, []string{"exchange", "result"})
)

// Processing outcome label values for amqp_messages_processed_total.
const (
	resultAck     = "ack"
	resultRequeue = "requeue"
	resultDrop    = "drop"
)

func init() {
	prometheus.MustRegister(messagesProcessed, messageProcessingSeconds, publishTotal)
}
