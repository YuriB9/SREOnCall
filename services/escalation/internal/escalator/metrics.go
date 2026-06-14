package escalator

import "github.com/prometheus/client_golang/prometheus"

// Escalation flow counters. The exhausted rate and on-call lookup failures are
// the on-call health signals worth alerting on (O2):
//
//	rate(escalation_exhausted_total[15m])              -> incidents nobody answered
//	rate(escalation_getoncall_failures_total[5m]) > 0  -> scheduling lookups failing
var (
	escalationsTriggered = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "escalation_triggered_total",
		Help: "Escalation tiers triggered (notification published)",
	})

	escalationsAdvanced = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "escalation_advanced_total",
		Help: "Escalations advanced to the next tier",
	})

	escalationsExhausted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "escalation_exhausted_total",
		Help: "Escalations exhausted with no remaining tiers",
	})

	getOnCallFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "escalation_getoncall_failures_total",
		Help: "Failed on-call lookups against the scheduling service",
	})
)

func init() {
	prometheus.MustRegister(escalationsTriggered, escalationsAdvanced, escalationsExhausted, getOnCallFailures)
}
