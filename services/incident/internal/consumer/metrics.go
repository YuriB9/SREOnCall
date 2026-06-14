package consumer

import "github.com/prometheus/client_golang/prometheus"

// Incident lifecycle counters, driven by the alerts.incident consumer:
//
//	rate(incident_incidents_created_total[5m])   -> new incidents per second
//	rate(incident_incidents_resolved_total[5m])  -> auto-resolutions per second
var (
	incidentsCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "incident_incidents_created_total",
		Help: "Incidents created from firing alerts",
	})

	incidentsResolved = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "incident_incidents_resolved_total",
		Help: "Incidents auto-resolved after all alerts cleared",
	})
)

func init() {
	prometheus.MustRegister(incidentsCreated, incidentsResolved)
}
