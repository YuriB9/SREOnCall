// Package monitor polls for expired escalation states and advances them, and
// exposes the backlog of pending states as a Prometheus gauge.
package monitor

import "github.com/prometheus/client_golang/prometheus"

// backlog is the number of expired escalation states seen on the latest tick.
// A persistently non-zero value means the monitor cannot keep up (a stuck
// pipeline or an overload), worth alerting on:
//
//	max_over_time(escalation_backlog[10m]) > 0
//
// It reflects the batch size capped by ListExpiredStates(limit); a value equal
// to the limit means the real backlog may be larger.
var backlog = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "escalation_backlog",
	Help: "Expired escalation states pending advancement on the latest monitor tick",
})

func init() {
	prometheus.MustRegister(backlog)
}
