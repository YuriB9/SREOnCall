package dedup

import (
	"context"
	"testing"
	"time"

	"github.com/sre-oncall/pkg/domain"
)

type memCache struct {
	data map[string]string
}

func newMemCache() *memCache { return &memCache{data: make(map[string]string)} }

func (m *memCache) SetNX(_ context.Context, key, val string, _ time.Duration) (bool, error) {
	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = val
	return true, nil
}

func (m *memCache) Del(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *memCache) Apply(ctx context.Context, setKeys []string, val string, ttl time.Duration, delKeys []string) ([]bool, error) {
	out := make([]bool, len(setKeys))
	for i, k := range setKeys {
		set, _ := m.SetNX(ctx, k, val, ttl)
		out[i] = set
	}
	for _, k := range delKeys {
		_ = m.Del(ctx, k)
	}
	return out, nil
}

func firingAlert(fp string) domain.Alert {
	return domain.Alert{
		TenantID:    "t",
		Fingerprint: fp,
		Status:      domain.AlertStatusFiring,
	}
}

func resolvedAlert(fp string) domain.Alert {
	a := firingAlert(fp)
	a.Status = domain.AlertStatusResolved
	return a
}

func TestIsDuplicate_NewAlert(t *testing.T) {
	d := New(newMemCache(), time.Hour)
	dup, err := d.IsDuplicate(context.Background(), firingAlert("fp1"))
	if err != nil {
		t.Fatal(err)
	}
	if dup {
		t.Error("first alert must not be a duplicate")
	}
}

func TestIsDuplicate_SecondCallIsDuplicate(t *testing.T) {
	d := New(newMemCache(), time.Hour)
	alert := firingAlert("fp2")
	d.IsDuplicate(context.Background(), alert) //nolint:errcheck
	dup, err := d.IsDuplicate(context.Background(), alert)
	if err != nil {
		t.Fatal(err)
	}
	if !dup {
		t.Error("second alert with same fingerprint must be a duplicate")
	}
}

func TestClear_AllowsRefire(t *testing.T) {
	d := New(newMemCache(), time.Hour)
	alert := firingAlert("fp3")
	d.IsDuplicate(context.Background(), alert) //nolint:errcheck

	if err := d.Clear(context.Background(), alert); err != nil {
		t.Fatal(err)
	}

	dup, _ := d.IsDuplicate(context.Background(), alert)
	if dup {
		t.Error("after Clear, alert must not be a duplicate")
	}
}

func TestIsDuplicate_DifferentFingerprints(t *testing.T) {
	d := New(newMemCache(), time.Hour)
	d.IsDuplicate(context.Background(), firingAlert("fp-a")) //nolint:errcheck
	dup, _ := d.IsDuplicate(context.Background(), firingAlert("fp-b"))
	if dup {
		t.Error("different fingerprints must not conflict")
	}
}

func TestClassify_MixedFiringAndResolved(t *testing.T) {
	t.Parallel()
	d := New(newMemCache(), time.Hour)
	// fp-x already seen (firing); resolved alerts always pass; new firing passes.
	d.IsDuplicate(context.Background(), firingAlert("fp-x")) //nolint:errcheck

	alerts := []domain.Alert{
		firingAlert("fp-x"),     // duplicate
		resolvedAlert("fp-res"), // resolved → never suppressed
		firingAlert("fp-new"),   // new → not duplicate
	}
	dup, err := d.Classify(context.Background(), alerts)
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, false}
	for i := range want {
		if dup[i] != want[i] {
			t.Errorf("alert %d: got dup=%v, want %v", i, dup[i], want[i])
		}
	}
}

func TestClassify_BatchDedupsSameFingerprint(t *testing.T) {
	t.Parallel()
	d := New(newMemCache(), time.Hour)
	alerts := []domain.Alert{firingAlert("fp-dup"), firingAlert("fp-dup")}
	dup, err := d.Classify(context.Background(), alerts)
	if err != nil {
		t.Fatal(err)
	}
	if dup[0] {
		t.Error("first occurrence in batch must not be a duplicate")
	}
	if !dup[1] {
		t.Error("second occurrence of same fingerprint in batch must be a duplicate")
	}
}

func TestClassify_Empty(t *testing.T) {
	t.Parallel()
	d := New(newMemCache(), time.Hour)
	dup, err := d.Classify(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(dup) != 0 {
		t.Errorf("expected empty result, got %d", len(dup))
	}
}
