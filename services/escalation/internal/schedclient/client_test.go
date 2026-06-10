package schedclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOnCallSendsAdminKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_id":"u1","username":"alice"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret")
	result, err := c.GetOnCall(context.Background(), "acme", "sched-1")
	if err != nil {
		t.Fatalf("GetOnCall: %v", err)
	}
	if gotKey != "secret" {
		t.Errorf("X-Admin-Key = %q, want %q", gotKey, "secret")
	}
	if result.UserID != "u1" || result.Username != "alice" {
		t.Errorf("result = %+v, want user_id=u1 username=alice", result)
	}
}

func TestGetOnCallUnauthorizedReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing authorization", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	result, err := c.GetOnCall(context.Background(), "acme", "sched-1")
	if err == nil {
		t.Fatalf("GetOnCall = %+v, want error on 401", result)
	}
}
