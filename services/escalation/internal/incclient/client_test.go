package incclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIncidentSendsAdminKey(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Admin-Key")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"inc-1","title":"DB on fire","severity":"critical","status":"open"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret")
	inc, err := c.GetIncident(context.Background(), "tenant-a", "inc-1")
	if err != nil {
		t.Fatalf("GetIncident: %v", err)
	}
	if gotKey != "secret" {
		t.Errorf("X-Admin-Key = %q, want %q", gotKey, "secret")
	}
	if gotPath != "/api/incidents/v1/tenant-a/incidents/inc-1" {
		t.Errorf("path = %q", gotPath)
	}
	if inc.Title != "DB on fire" || inc.Severity != "critical" || inc.Status != "open" {
		t.Errorf("incident = %+v", inc)
	}
}

func TestGetIncidentErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing authorization", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	if inc, err := c.GetIncident(context.Background(), "tenant-a", "inc-1"); err == nil {
		t.Fatalf("GetIncident = %+v, want error on 401", inc)
	}
}
