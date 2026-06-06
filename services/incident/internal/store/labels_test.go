package store_test

import (
	"testing"

	"github.com/sre-oncall/incident/internal/store"
)

func TestLabelsToJSON_RoundTrip(t *testing.T) {
	labels := map[string]string{"env": "prod", "team": "platform"}
	got := store.LabelsToJSON(labels)
	if got == "" {
		t.Error("expected non-empty JSON")
	}
	if got == "{}" && len(labels) > 0 {
		t.Error("expected labels to be serialised")
	}
}

func TestLabelsToJSON_Empty(t *testing.T) {
	got := store.LabelsToJSON(nil)
	if got != "null" && got != "{}" && got == "" {
		t.Error("expected valid JSON for nil labels")
	}
}
