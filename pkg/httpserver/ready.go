package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// readyTimeout bounds the whole readiness evaluation so a hung dependency
// cannot make /readyz block indefinitely.
const readyTimeout = 3 * time.Second

// errUnavailable is the generic failure reported by BoolCheck. The /readyz
// response never surfaces the reason text to the client, only the 503 status.
var errUnavailable = errors.New("dependency unavailable")

// Check is a single named readiness probe. Probe returns nil when the
// dependency is reachable and an error otherwise.
type Check struct {
	Name  string
	Probe func(ctx context.Context) error
}

// BoolCheck adapts a boolean liveness signal (e.g. amqp.Connection.Ready or a
// consumer's Healthy) into a Check.
func BoolCheck(name string, ok func() bool) Check {
	return Check{Name: name, Probe: func(context.Context) error {
		if ok() {
			return nil
		}
		return errUnavailable
	}}
}

// ReadyHandler builds the /readyz handler from a set of dependency checks. It
// runs every check under a shared timeout and returns 200 only when all pass;
// any failure yields 503 so Kubernetes pulls the pod out of rotation (O1). The
// JSON body reports each check as "ok"/"down" (failure detail is not leaked to
// the client — it is reflected only in the HTTP status; cf. E6). With no checks
// the endpoint behaves like the previous static 200.
func ReadyHandler(checks ...Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		results := make(map[string]string, len(checks))
		ready := true
		for _, c := range checks {
			if err := c.Probe(ctx); err != nil {
				results[c.Name] = "down"
				ready = false
			} else {
				results[c.Name] = "ok"
			}
		}

		status := http.StatusOK
		state := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			state = "unavailable"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": state,
			"checks": results,
		})
	}
}
