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

func firingAlert(fp string) domain.Alert {
	return domain.Alert{
		TenantID:    "t",
		Fingerprint: fp,
		Status:      domain.AlertStatusFiring,
	}
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
