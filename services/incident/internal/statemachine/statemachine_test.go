package statemachine_test

import (
	"testing"

	incdomain "github.com/sre-oncall/incident/internal/domain"
	"github.com/sre-oncall/incident/internal/statemachine"
)

func TestValidate_AllowedTransitions(t *testing.T) {
	cases := []struct {
		from incdomain.Status
		to   incdomain.Status
	}{
		{incdomain.StatusOpen, incdomain.StatusAcknowledged},
		{incdomain.StatusOpen, incdomain.StatusResolved},
		{incdomain.StatusAcknowledged, incdomain.StatusResolved},
		{incdomain.StatusResolved, incdomain.StatusOpen},
	}
	for _, tc := range cases {
		if err := statemachine.Validate(tc.from, tc.to); err != nil {
			t.Errorf("expected %s → %s to be allowed, got: %v", tc.from, tc.to, err)
		}
	}
}

func TestValidate_ForbiddenTransitions(t *testing.T) {
	cases := []struct {
		from incdomain.Status
		to   incdomain.Status
	}{
		{incdomain.StatusOpen, incdomain.StatusOpen},
		{incdomain.StatusAcknowledged, incdomain.StatusOpen},
		{incdomain.StatusAcknowledged, incdomain.StatusAcknowledged},
		{incdomain.StatusResolved, incdomain.StatusAcknowledged},
		{incdomain.StatusResolved, incdomain.StatusResolved},
	}
	for _, tc := range cases {
		if err := statemachine.Validate(tc.from, tc.to); err == nil {
			t.Errorf("expected %s → %s to be forbidden, got nil", tc.from, tc.to)
		}
	}
}

func TestValidate_ErrorType(t *testing.T) {
	err := statemachine.Validate(incdomain.StatusAcknowledged, incdomain.StatusOpen)
	var inv statemachine.ErrInvalidTransition
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(statemachine.ErrInvalidTransition); !ok {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
	_ = inv
}
