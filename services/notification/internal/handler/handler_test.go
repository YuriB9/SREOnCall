package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sre-oncall/notification/internal/domain"
	"github.com/sre-oncall/notification/internal/store"
	pkgauth "github.com/sre-oncall/pkg/auth"
)

// fakeStore implements the handler Store interface in memory.
type fakeStore struct {
	contacts map[string]*domain.UserContact // key: tenant + ":" + user
	upserts  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{contacts: make(map[string]*domain.UserContact)}
}

func key(tenant, user string) string { return tenant + ":" + user }

func (f *fakeStore) GetContact(_ context.Context, tenant, user string) (*domain.UserContact, error) {
	if c, ok := f.contacts[key(tenant, user)]; ok {
		clone := *c
		return &clone, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) UpsertContact(_ context.Context, c *domain.UserContact) error {
	f.upserts++
	c.ID = "id-" + key(c.TenantID, c.UserID)
	stored := *c
	f.contacts[key(c.TenantID, c.UserID)] = &stored
	return nil
}

func newTestHandler(st Store) *Handler {
	return New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func syncRequestWithClaims(claims pkgauth.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/notifications/v1/sync-contacts", nil)
	return r.WithContext(pkgauth.WithClaims(r.Context(), claims))
}

func TestTenantSlugs(t *testing.T) {
	got := tenantSlugs([]string{"/team-a", "/team-a/admins", "/team-b", "", "/"})
	want := []string{"team-a", "team-b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSyncContactsCreatesAndUpdates(t *testing.T) {
	st := newFakeStore()
	// Pre-existing contact in team-a with a stale email and a custom channel set.
	st.contacts[key("team-a", "u1")] = &domain.UserContact{
		ID: "existing", UserID: "u1", TenantID: "team-a",
		Email: "old@example.com", MattermostUsername: "u1mm",
		EnabledChannels: []string{"mattermost"},
	}
	h := newTestHandler(st)

	claims := pkgauth.Claims{
		Sub:    "u1",
		Email:  "new@example.com",
		Groups: []string{"/team-a", "/team-b/admins"},
	}
	rr := httptest.NewRecorder()
	h.SyncContacts(rr, syncRequestWithClaims(claims))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}

	// team-a: email updated to token value, other fields preserved.
	a := st.contacts[key("team-a", "u1")]
	if a.Email != "new@example.com" {
		t.Errorf("team-a email = %q, want new@example.com", a.Email)
	}
	if a.MattermostUsername != "u1mm" || len(a.EnabledChannels) != 1 || a.EnabledChannels[0] != "mattermost" {
		t.Errorf("team-a other fields not preserved: %+v", a)
	}

	// team-b: created with email channel enabled.
	b, ok := st.contacts[key("team-b", "u1")]
	if !ok {
		t.Fatal("team-b contact not created")
	}
	if b.Email != "new@example.com" || len(b.EnabledChannels) != 1 || b.EnabledChannels[0] != "email" {
		t.Errorf("team-b contact = %+v", b)
	}

	var resp map[string]int
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["synced"] != 2 {
		t.Errorf("synced = %d, want 2", resp["synced"])
	}
}

func TestSyncContactsNoChangeWhenEmailMatches(t *testing.T) {
	st := newFakeStore()
	st.contacts[key("team-a", "u1")] = &domain.UserContact{
		ID: "existing", UserID: "u1", TenantID: "team-a",
		Email: "same@example.com", EnabledChannels: []string{"email"},
	}
	h := newTestHandler(st)

	claims := pkgauth.Claims{Sub: "u1", Email: "same@example.com", Groups: []string{"/team-a"}}
	rr := httptest.NewRecorder()
	h.SyncContacts(rr, syncRequestWithClaims(claims))

	if st.upserts != 0 {
		t.Errorf("expected no upserts, got %d", st.upserts)
	}
}

func TestSyncContactsNoEmailClaim(t *testing.T) {
	st := newFakeStore()
	h := newTestHandler(st)
	claims := pkgauth.Claims{Sub: "u1", Groups: []string{"/team-a"}}
	rr := httptest.NewRecorder()
	h.SyncContacts(rr, syncRequestWithClaims(claims))
	if st.upserts != 0 {
		t.Errorf("expected no upserts without email, got %d", st.upserts)
	}
}
