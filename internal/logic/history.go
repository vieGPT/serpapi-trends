package logic

import (
	"fmt"

	"github.com/vieGPT/serpapi-trends/internal/store"
)

// HistoryHit is a single FTS match from local snapshots.
type HistoryHit struct {
	Engine     string
	ParamsJSON string
	CreatedAt  string // RFC3339 for easy JSON
	Snippet    string // truncated response for display
}

// SearchHistory runs FTS over the local snapshot store.
// Pure local; no live API.
func SearchHistory(s *store.Store, query string, limit int) ([]HistoryHit, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if query == "" {
		return nil, fmt.Errorf("query required")
	}
	sns, err := s.SearchFTS(query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryHit, 0, len(sns))
	for _, sn := range sns {
		snip := sn.ResponseJSON
		if len(snip) > 200 {
			snip = snip[:200] + "…"
		}
		out = append(out, HistoryHit{
			Engine:     sn.Engine,
			ParamsJSON: sn.ParamsJSON,
			CreatedAt:  sn.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Snippet:    snip,
		})
	}
	return out, nil
}
