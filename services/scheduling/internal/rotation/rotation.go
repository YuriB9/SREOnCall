package rotation

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sre-oncall/scheduling/internal/domain"
)

var ErrEmptyRotation = errors.New("rotation has no members")

// ParseISO8601Duration parses a subset of ISO 8601 duration strings.
// Supports: P<n>D, P<n>W, PT<n>H, PT<n>M, P<n>DT<n>H.
func ParseISO8601Duration(s string) (time.Duration, error) {
	if s == "" || s[0] != 'P' {
		return 0, fmt.Errorf("invalid duration: %q", s)
	}
	s = s[1:] // strip leading P

	var total time.Duration
	inTime := false

	for len(s) > 0 {
		if s[0] == 'T' {
			inTime = true
			s = s[1:]
			continue
		}
		// Find numeric prefix
		i := 0
		for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
			i++
		}
		if i == 0 || i >= len(s) {
			return 0, fmt.Errorf("invalid duration: unexpected char %q", s)
		}
		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, err
		}
		unit := s[i]
		s = s[i+1:]

		switch {
		case !inTime && unit == 'W':
			total += time.Duration(n) * 7 * 24 * time.Hour
		case !inTime && unit == 'D':
			total += time.Duration(n) * 24 * time.Hour
		case inTime && unit == 'H':
			total += time.Duration(n) * time.Hour
		case inTime && unit == 'M':
			total += time.Duration(n) * time.Minute
		case inTime && unit == 'S':
			total += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("invalid duration unit %q", unit)
		}
	}
	if total <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return total, nil
}

// OnCallAt returns the user ID on call for schedule s at timestamp at.
// Overrides take priority over the rotation.
func OnCallAt(s *domain.Schedule, overrides []*domain.Override, at time.Time) (string, time.Time, time.Time, error) {
	// Check overrides first
	for _, o := range overrides {
		if !at.Before(o.StartAt) && at.Before(o.EndAt) {
			return o.UserID, o.StartAt, o.EndAt, nil
		}
	}
	return fromRotation(s, at)
}

// fromRotation computes the on-call user from the rotation at time at.
func fromRotation(s *domain.Schedule, at time.Time) (string, time.Time, time.Time, error) {
	if len(s.Rotation) == 0 {
		return "", time.Time{}, time.Time{}, ErrEmptyRotation
	}

	loc, err := time.LoadLocation(strings.TrimSpace(s.Timezone))
	if err != nil {
		loc = time.UTC
	}

	shiftDur, err := ParseISO8601Duration(s.ShiftDuration)
	if err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("parse shift_duration: %w", err)
	}

	// Epoch = start of start_date in schedule timezone
	epoch := time.Date(s.StartDate.Year(), s.StartDate.Month(), s.StartDate.Day(), 0, 0, 0, 0, loc)

	atLocal := at.In(loc)
	elapsed := atLocal.Sub(epoch)
	if elapsed < 0 {
		elapsed = 0
	}

	shiftIndex := int(elapsed/shiftDur) % len(s.Rotation)
	userID := s.Rotation[shiftIndex]

	shiftStart := epoch.Add(time.Duration(int(elapsed/shiftDur)) * shiftDur)
	shiftEnd := shiftStart.Add(shiftDur)

	return userID, shiftStart.UTC(), shiftEnd.UTC(), nil
}

// GenerateShifts produces all shifts (rotation + overrides) within [from, to).
func GenerateShifts(s *domain.Schedule, overrides []*domain.Override, from, to time.Time) ([]domain.Shift, error) {
	if len(s.Rotation) == 0 {
		return nil, ErrEmptyRotation
	}

	loc, err := time.LoadLocation(strings.TrimSpace(s.Timezone))
	if err != nil {
		loc = time.UTC
	}

	shiftDur, err := ParseISO8601Duration(s.ShiftDuration)
	if err != nil {
		return nil, fmt.Errorf("parse shift_duration: %w", err)
	}

	epoch := time.Date(s.StartDate.Year(), s.StartDate.Month(), s.StartDate.Day(), 0, 0, 0, 0, loc)

	// Build base rotation shifts
	var shifts []domain.Shift
	cur := from
	for cur.Before(to) {
		elapsed := cur.In(loc).Sub(epoch)
		if elapsed < 0 {
			elapsed = 0
		}
		idx := int(elapsed/shiftDur) % len(s.Rotation)
		shiftStart := epoch.Add(time.Duration(int(elapsed/shiftDur)) * shiftDur)
		shiftEnd := shiftStart.Add(shiftDur)
		if shiftEnd.After(to) {
			shiftEnd = to
		}
		shifts = append(shifts, domain.Shift{
			UserID:  s.Rotation[idx],
			StartAt: shiftStart.UTC(),
			EndAt:   shiftEnd.UTC(),
		})
		cur = shiftEnd
	}

	// Apply overrides: cut rotation shifts and insert override windows
	shifts = applyOverrides(shifts, overrides, from, to)
	return shifts, nil
}

func applyOverrides(base []domain.Shift, overrides []*domain.Override, from, to time.Time) []domain.Shift {
	if len(overrides) == 0 {
		return base
	}

	result := make([]domain.Shift, 0, len(base)+len(overrides))
	for _, s := range base {
		cur := s.StartAt
		for _, o := range overrides {
			// Clamp override window to [from, to)
			oStart := o.StartAt
			oEnd := o.EndAt
			if oStart.Before(from) {
				oStart = from
			}
			if oEnd.After(to) {
				oEnd = to
			}
			if !oStart.Before(oEnd) {
				continue
			}
			// If override overlaps this base shift
			if oStart.Before(s.EndAt) && oEnd.After(s.StartAt) {
				// Rotation part before override
				if cur.Before(oStart) {
					result = append(result, domain.Shift{UserID: s.UserID, StartAt: cur, EndAt: oStart})
				}
				// Override window
				result = append(result, domain.Shift{UserID: o.UserID, StartAt: oStart, EndAt: oEnd, IsOverride: true})
				cur = oEnd
			}
		}
		// Remaining rotation part
		if cur.Before(s.EndAt) {
			result = append(result, domain.Shift{UserID: s.UserID, StartAt: cur, EndAt: s.EndAt})
		}
	}
	return result
}
