// Package notifier dispatches escalation notifications to users over the
// enabled channels (email, Mattermost) and records delivery metrics.
package notifier

import "github.com/prometheus/client_golang/prometheus"

// Notification delivery counters — the "do alerts actually reach people" signal:
//
//	sum(rate(notification_sent_total{result="failed"}[5m])) by (channel)
//	  / sum(rate(notification_sent_total[5m])) by (channel)   -> failure ratio
//	rate(notification_rate_limited_total[5m])                 -> throttling pressure
var (
	notificationsSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_sent_total",
		Help: "Notification dispatch attempts, by channel and result (delivered|failed)",
	}, []string{"channel", "result"})

	notificationsRateLimited = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_rate_limited_total",
		Help: "Notifications suppressed by the per-user rate limiter, by channel",
	}, []string{"channel"})
)

// Result label values for notification_sent_total.
const (
	resultDelivered = "delivered"
	resultFailed    = "failed"
)

func init() {
	prometheus.MustRegister(notificationsSent, notificationsRateLimited)
}
