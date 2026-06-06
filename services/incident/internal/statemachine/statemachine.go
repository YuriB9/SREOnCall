package statemachine

import (
	"fmt"

	"github.com/sre-oncall/incident/internal/domain"
)

// ErrInvalidTransition is returned when a status transition is not allowed.
type ErrInvalidTransition struct {
	From domain.Status
	To   domain.Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid transition: %s → %s", e.From, e.To)
}

// Validate returns nil if transitioning from `from` to `to` is allowed.
// Allowed:
//   - open → acknowledged
//   - open → resolved
//   - acknowledged → resolved
//   - resolved → open  (reopen)
func Validate(from, to domain.Status) error {
	switch from {
	case domain.StatusOpen:
		if to == domain.StatusAcknowledged || to == domain.StatusResolved {
			return nil
		}
	case domain.StatusAcknowledged:
		if to == domain.StatusResolved {
			return nil
		}
	case domain.StatusResolved:
		if to == domain.StatusOpen {
			return nil
		}
	}
	return ErrInvalidTransition{From: from, To: to}
}
