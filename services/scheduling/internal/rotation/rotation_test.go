package rotation_test

import (
	"testing"
	"time"

	"github.com/sre-oncall/scheduling/internal/domain"
	"github.com/sre-oncall/scheduling/internal/rotation"
)

func makeSchedule(rotation []string, shiftDur, tz, startDate string) *domain.Schedule {
	sd, _ := time.Parse("2006-01-02", startDate)
	return &domain.Schedule{
		ID:            "sched1",
		TenantID:      "t",
		Rotation:      rotation,
		ShiftDuration: shiftDur,
		Timezone:      tz,
		StartDate:     sd,
	}
}

// ── ParseISO8601Duration ──────────────────────────────────────────────────────

func TestParseISO8601Duration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  time.Duration
		fail  bool
	}{
		{"7 days", "P7D", 7 * 24 * time.Hour, false},
		{"1 week", "P1W", 7 * 24 * time.Hour, false},
		{"14 days", "P14D", 14 * 24 * time.Hour, false},
		{"12 hours", "PT12H", 12 * time.Hour, false},
		{"30 minutes", "PT30M", 30 * time.Minute, false},
		{"day and time", "P1DT12H", 36 * time.Hour, false},
		{"empty string", "", 0, true},
		{"missing P prefix", "7D", 0, true},
		{"P only", "P", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := rotation.ParseISO8601Duration(c.input)
			if c.fail {
				if err == nil {
					t.Errorf("ParseISO8601Duration(%q) expected error, got nil", c.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseISO8601Duration(%q) unexpected error: %v", c.input, err)
				return
			}
			if got != c.want {
				t.Errorf("ParseISO8601Duration(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// ── OnCallAt — basic rotation ─────────────────────────────────────────────────

func TestOnCallAt_BasicRotation(t *testing.T) {
	t.Parallel()
	sched := makeSchedule([]string{"alice", "bob", "carol"}, "P7D", "UTC", "2024-01-01")
	// Week 0: alice
	at := time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC)
	userID, _, _, err := rotation.OnCallAt(sched, nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "alice" {
		t.Errorf("expected alice, got %s", userID)
	}

	// Week 1: bob
	at = time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	userID, _, _, err = rotation.OnCallAt(sched, nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "bob" {
		t.Errorf("expected bob, got %s", userID)
	}

	// Week 3: alice (wraps around)
	at = time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
	userID, _, _, err = rotation.OnCallAt(sched, nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "alice" {
		t.Errorf("expected alice (wrap), got %s", userID)
	}
}

// ── OnCallAt — override priority ──────────────────────────────────────────────

func TestOnCallAt_OverridePriority(t *testing.T) {
	t.Parallel()
	sched := makeSchedule([]string{"alice", "bob"}, "P7D", "UTC", "2024-01-01")

	// Override: dave is on call for the full first week
	override := &domain.Override{
		ID:         "o1",
		ScheduleID: sched.ID,
		TenantID:   sched.TenantID,
		UserID:     "dave",
		StartAt:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndAt:      time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
	}

	at := time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC) // in override window
	userID, _, _, err := rotation.OnCallAt(sched, []*domain.Override{override}, at)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "dave" {
		t.Errorf("expected dave (override), got %s", userID)
	}

	// After override: back to rotation (alice at week 1 = bob)
	at = time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC) // week 1 = bob
	userID, _, _, err = rotation.OnCallAt(sched, []*domain.Override{override}, at)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "bob" {
		t.Errorf("expected bob after override, got %s", userID)
	}
}

// ── OnCallAt — DST edge case ──────────────────────────────────────────────────

func TestOnCallAt_DST(t *testing.T) {
	t.Parallel()
	// Europe/Berlin switches DST in late March; shift boundary must not break
	sched := makeSchedule([]string{"alice", "bob"}, "P7D", "Europe/Berlin", "2024-03-25")

	// 2024-03-31: clocks go forward at 02:00 (DST switch)
	loc, _ := time.LoadLocation("Europe/Berlin")
	atDST := time.Date(2024, 3, 31, 12, 0, 0, 0, loc)
	userID, _, _, err := rotation.OnCallAt(sched, nil, atDST)
	if err != nil {
		t.Fatalf("DST rotation failed: %v", err)
	}
	// Should still resolve a user without panic
	if userID != "alice" && userID != "bob" {
		t.Errorf("unexpected user during DST week: %s", userID)
	}
}

// ── GenerateShifts ────────────────────────────────────────────────────────────

func TestGenerateShifts_NoOverrides(t *testing.T) {
	t.Parallel()
	sched := makeSchedule([]string{"alice", "bob"}, "P7D", "UTC", "2024-01-01")
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	shifts, err := rotation.GenerateShifts(sched, nil, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(shifts) < 2 {
		t.Errorf("expected at least 2 shifts, got %d", len(shifts))
	}
	if shifts[0].UserID != "alice" {
		t.Errorf("first shift: expected alice, got %s", shifts[0].UserID)
	}
	if shifts[1].UserID != "bob" {
		t.Errorf("second shift: expected bob, got %s", shifts[1].UserID)
	}
}

func TestGenerateShifts_WithOverride(t *testing.T) {
	t.Parallel()
	sched := makeSchedule([]string{"alice", "bob"}, "P7D", "UTC", "2024-01-01")
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	override := &domain.Override{
		UserID:  "dave",
		StartAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
	}

	shifts, err := rotation.GenerateShifts(sched, []*domain.Override{override}, from, to)
	if err != nil {
		t.Fatal(err)
	}

	// Find override shift
	var found bool
	for _, s := range shifts {
		if s.IsOverride && s.UserID == "dave" {
			found = true
		}
	}
	if !found {
		t.Error("expected override shift for dave, not found")
	}
}

func TestOnCallAt_EmptyRotation(t *testing.T) {
	t.Parallel()
	sched := makeSchedule([]string{}, "P7D", "UTC", "2024-01-01")
	_, _, _, err := rotation.OnCallAt(sched, nil, time.Now())
	if err == nil {
		t.Error("expected error for empty rotation")
	}
}
