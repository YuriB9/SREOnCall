package store

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeConn records Exec calls so we can assert mergeLabels issues a single
// multi-row upsert instead of one INSERT per label.
type fakeConn struct {
	execs []execCall
}

type execCall struct {
	sql  string
	args []any
}

func (f *fakeConn) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execs = append(f.execs, execCall{sql: sql, args: args})
	return pgconn.CommandTag{}, nil
}

func (f *fakeConn) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return nil, nil }
func (f *fakeConn) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row        { return nil }

func TestMergeLabels_SingleMultiRowUpsert(t *testing.T) {
	t.Parallel()
	f := &fakeConn{}
	labels := map[string]string{"env": "prod", "team": "platform", "severity": "high"}

	if err := mergeLabels(context.Background(), f, "inc-1", labels); err != nil {
		t.Fatal(err)
	}

	if len(f.execs) != 1 {
		t.Fatalf("expected exactly 1 Exec for %d labels, got %d", len(labels), len(f.execs))
	}
	call := f.execs[0]
	if !strings.Contains(call.sql, "unnest") {
		t.Errorf("expected unnest-based multi-row insert, got SQL:\n%s", call.sql)
	}
	if !strings.Contains(call.sql, "ON CONFLICT") {
		t.Errorf("expected ON CONFLICT upsert, got SQL:\n%s", call.sql)
	}
	if len(call.args) != 3 {
		t.Fatalf("expected 3 args (incidentID, keys, values), got %d", len(call.args))
	}
	keys, ok := call.args[1].([]string)
	if !ok || len(keys) != len(labels) {
		t.Errorf("expected keys []string of len %d, got %T len %d", len(labels), call.args[1], len(keys))
	}
	values, ok := call.args[2].([]string)
	if !ok || len(values) != len(labels) {
		t.Errorf("expected values []string of len %d, got %T len %d", len(labels), call.args[2], len(values))
	}
}

func TestMergeLabels_EmptyNoOp(t *testing.T) {
	t.Parallel()
	f := &fakeConn{}
	if err := mergeLabels(context.Background(), f, "inc-1", nil); err != nil {
		t.Fatal(err)
	}
	if len(f.execs) != 0 {
		t.Errorf("expected no Exec for empty labels, got %d", len(f.execs))
	}
}
