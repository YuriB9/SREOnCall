package handler

import "github.com/prometheus/client_golang/prometheus"

// alertsReceived counts every alert accepted from a webhook, by source, before
// deduplication. Together with ingestion_dedup_* it answers "how many alerts
// come in and how many survive dedup":
//
//	rate(ingestion_alerts_received_total[5m])  -> inbound alert rate by source
var alertsReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "ingestion_alerts_received_total",
	Help: "Alerts received from webhooks, before deduplication, by source",
}, []string{"source"})

func init() {
	prometheus.MustRegister(alertsReceived)
}
