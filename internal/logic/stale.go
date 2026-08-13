// Package logic holds pure local computations over the snapshot store.
// No live API calls. Real table-driven tests required.
package logic

import (
	"fmt"
	"time"

	"github.com/vieGPT/serpapi-trends/internal/store"
)

// StaleResult is one entry from a stale scan.
type StaleResult struct {
	Engine     string
	ParamsHash string
	ParamsJSON string
	Age        time.Duration
	CreatedAt  time.Time
}

// FindStale returns latest snapshots older than maxAge.
// Pure local; does not call SerpAPI.
func FindStale(s *store.Store, maxAge time.Duration) ([]StaleResult, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if maxAge <= 0 {
		return nil, fmt.Errorf("maxAge must be positive")
	}
	sns, err := s.ListStale(maxAge)
	if err != nil {
		return nil, err
	}
	out := make([]StaleResult, 0, len(sns))
	now := time.Now()
	for _, sn := range sns {
		out = append(out, StaleResult{
			Engine:     sn.Engine,
			ParamsHash: sn.ParamsHash,
			ParamsJSON: sn.ParamsJSON,
			Age:        now.Sub(sn.CreatedAt),
			CreatedAt:  sn.CreatedAt,
		})
	}
	return out, nil
}

// IsStale reports whether the latest snapshot for engine+params is older than maxAge.
// Returns (true, age, nil) if stale or missing; (false, age, nil) if fresh.
func IsStale(s *store.Store, engine string, params map[string]string, maxAge time.Duration) (bool, time.Duration, error) {
	if s == nil {
		return true, 0, fmt.Errorf("store is nil")
	}
	age, ok, err := s.Age(engine, params)
	if err != nil {
		return true, 0, err
	}
	if !ok {
		return true, 0, nil // missing counts as stale
	}
	return age > maxAge, age, nil
}
