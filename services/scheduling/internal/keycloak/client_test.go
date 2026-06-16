package keycloak_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sre-oncall/scheduling/internal/keycloak"
)

// fakeKeycloak отвечает на эндпоинты Admin REST API, которые дёргает GetMembers:
// token (client_credentials) → поиск группы по path → участники → подгруппа
// admins → участники admins. Группа acme: alice (member), bob (admin).
func fakeKeycloak(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/realms/test/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("token: method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-123"}`))
	})

	mux.HandleFunc("/admin/realms/test/groups", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			t.Errorf("groups: Authorization = %q, want Bearer tok-123", got)
		}
		_, _ = w.Write([]byte(`[{"id":"g1","name":"acme","path":"/acme"}]`))
	})

	mux.HandleFunc("/admin/realms/test/groups/g1/members", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"u1","username":"alice"},{"id":"u2","username":"bob"}]`))
	})

	mux.HandleFunc("/admin/realms/test/groups/g1/children", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"g2","name":"admins","path":"/acme/admins"}]`))
	})

	mux.HandleFunc("/admin/realms/test/groups/g2/members", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"u2","username":"bob"}]`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetMembers_AssignsRoles(t *testing.T) {
	t.Parallel()
	srv := fakeKeycloak(t)
	c := keycloak.New(srv.URL, "test", "admin-cli", "secret")

	members, err := c.GetMembers(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}

	roles := map[string]string{}
	for _, m := range members {
		roles[m.PreferredUsername] = m.Role
	}
	if roles["alice"] != "member" {
		t.Errorf("alice role = %q, want member", roles["alice"])
	}
	if roles["bob"] != "admin" {
		t.Errorf("bob role = %q, want admin (in /acme/admins)", roles["bob"])
	}
}

func TestGetMembers_TokenError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/test/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid_client", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := keycloak.New(srv.URL, "test", "admin-cli", "bad-secret")
	_, err := c.GetMembers(context.Background(), "acme")
	if err == nil {
		t.Fatal("expected error on token failure")
	}
	if !strings.Contains(err.Error(), "get token") {
		t.Errorf("error should mention token, got: %v", err)
	}
}

func TestGetMembers_GroupNotFound(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/test/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok"}`))
	})
	mux.HandleFunc("/admin/realms/test/groups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`)) // тенант-группы нет
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := keycloak.New(srv.URL, "test", "admin-cli", "secret")
	_, err := c.GetMembers(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error when group is missing")
	}
}
