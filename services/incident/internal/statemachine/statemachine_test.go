package statemachine_test

import (
	"testing"

	incdomain "github.com/sre-oncall/incident/internal/domain"
	"github.com/sre-oncall/incident/internal/statemachine"
)

// TestValidate_TransitionMatrix покрывает матрицу переходов стейт-машины
// инцидента именованными подтестами: имя кейса в выводе сразу показывает, какой
// переход упал, и добавить новый кейс — одна строка.
func TestValidate_TransitionMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		from    incdomain.Status
		to      incdomain.Status
		allowed bool
	}{
		{"open → acknowledged", incdomain.StatusOpen, incdomain.StatusAcknowledged, true},
		{"open → resolved", incdomain.StatusOpen, incdomain.StatusResolved, true},
		{"acknowledged → resolved", incdomain.StatusAcknowledged, incdomain.StatusResolved, true},
		{"resolved → open (reopen)", incdomain.StatusResolved, incdomain.StatusOpen, true},

		{"open → open", incdomain.StatusOpen, incdomain.StatusOpen, false},
		{"acknowledged → open", incdomain.StatusAcknowledged, incdomain.StatusOpen, false},
		{"acknowledged → acknowledged", incdomain.StatusAcknowledged, incdomain.StatusAcknowledged, false},
		{"resolved → acknowledged", incdomain.StatusResolved, incdomain.StatusAcknowledged, false},
		{"resolved → resolved", incdomain.StatusResolved, incdomain.StatusResolved, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := statemachine.Validate(tc.from, tc.to)
			if tc.allowed && err != nil {
				t.Errorf("expected %s → %s to be allowed, got: %v", tc.from, tc.to, err)
			}
			if !tc.allowed && err == nil {
				t.Errorf("expected %s → %s to be forbidden, got nil", tc.from, tc.to)
			}
		})
	}
}

func TestValidate_ErrorType(t *testing.T) {
	t.Parallel()
	err := statemachine.Validate(incdomain.StatusAcknowledged, incdomain.StatusOpen)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(statemachine.ErrInvalidTransition); !ok {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
}
