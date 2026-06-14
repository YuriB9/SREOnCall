package store

import (
	"encoding/base64"
	"testing"
	"time"
)

func mustB64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func TestCursor_RoundTrip(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 6, 14, 12, 30, 45, 123456789, time.UTC)
	id := "11111111-2222-3333-4444-555555555555"

	cur := encodeCursor(created, id)
	if cur == "" {
		t.Fatal("expected non-empty cursor")
	}

	gotTime, gotID, ok := decodeCursor(cur)
	if !ok {
		t.Fatal("expected cursor to decode")
	}
	if !gotTime.Equal(created) {
		t.Errorf("created_at round-trip mismatch: got %v want %v", gotTime, created)
	}
	if gotID != id {
		t.Errorf("id round-trip mismatch: got %q want %q", gotID, id)
	}
}

func TestDecodeCursor_InvalidIsFirstPage(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"empty":         "",
		"not base64":    "@@@not-base64@@@",
		"missing sep":   mustB64("no-separator-here"),
		"empty id":      mustB64("2026-06-14T12:30:45Z|"),
		"bad timestamp": mustB64("not-a-time|some-id"),
	}
	for name, cur := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, ok := decodeCursor(cur); ok {
				t.Errorf("expected %q to be treated as first page (ok=false)", name)
			}
		})
	}
}
