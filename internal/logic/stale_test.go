package logic

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vieGPT/serpapi-trends/internal/store"
)

func openTempStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestIsStale_table(t *testing.T) {
	s := openTempStore(t)
	params := map[string]string{"q": "enoch", "geo": "US"}
	engine := "google_trends"

	// No snapshot yet → stale
	stale, age, err := IsStale(s, engine, params, time.Hour)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !stale {
		t.Fatalf("expected stale when missing, age=%v", age)
	}

	// Save a fresh snapshot
	resp := map[string]interface{}{"search_metadata": map[string]interface{}{"status": "Success"}}
	if err := s.Save(engine, params, resp); err != nil {
		t.Fatalf("save: %v", err)
	}

	stale, age, err = IsStale(s, engine, params, time.Hour)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stale {
		t.Fatalf("expected fresh, age=%v", age)
	}
	if age < 0 || age > time.Minute {
		t.Fatalf("unexpected age for fresh snapshot: %v", age)
	}

	// Very short maxAge → should be stale
	stale, _, err = IsStale(s, engine, params, time.Nanosecond)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !stale {
		t.Fatal("expected stale with tiny maxAge")
	}
}

func TestFindStale_table(t *testing.T) {
	tests := []struct {
		name    string
		maxAge  time.Duration
		setup   func(*store.Store)
		wantLen int
		wantErr bool
	}{
		{
			name:    "nil maxAge rejected via zero",
			maxAge:  0,
			wantErr: true,
		},
		{
			name:    "empty store",
			maxAge:  time.Hour,
			wantLen: 0,
		},
		{
			name:   "one fresh snapshot",
			maxAge: time.Hour,
			setup: func(st *store.Store) {
				_ = st.Save("google_trends", map[string]string{"q": "a"}, map[string]interface{}{"ok": true})
			},
			wantLen: 0, // fresh → not returned by ListStale
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := openTempStore(t)
			if tt.setup != nil {
				tt.setup(st)
			}
			got, err := FindStale(st, tt.maxAge)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len=%d want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParamsHash_stable(t *testing.T) {
	a := store.ParamsHash(map[string]string{"b": "2", "a": "1", "api_key": "secret"})
	b := store.ParamsHash(map[string]string{"a": "1", "b": "2"})
	if a != b {
		t.Fatalf("hash not stable or not ignoring api_key: %s vs %s", a, b)
	}
	if a == "" {
		t.Fatal("empty hash")
	}
}
