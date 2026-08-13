package logic

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/vieGPT/serpapi-trends/internal/store"
)

// Opportunity ranks a related/trending term against parent context.
type Opportunity struct {
	Query              string  `json:"query"`
	IncreasePercentage float64 `json:"increase_percentage,omitempty"`
	SearchVolume       float64 `json:"search_volume,omitempty"`
	Source             string  `json:"source"` // related_rising | trending
	ParentQ            string  `json:"parent_q,omitempty"`
	Score              float64 `json:"score"`
}

// ChangesDiff is a simple before/after related-query set diff.
type ChangesDiff struct {
	ParentQ   string   `json:"parent_q"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Unchanged []string `json:"unchanged"`
	FromID    int64    `json:"from_id,omitempty"`
	ToID      int64    `json:"to_id,omitempty"`
}

// GeoGapEntry is a region with notable relative interest.
type GeoGapEntry struct {
	Geo            string  `json:"geo"`
	Location       string  `json:"location,omitempty"`
	ExtractedValue float64 `json:"extracted_value"`
	Query          string  `json:"query,omitempty"`
}

// SeasonalityPoint is a coarse monthly aggregate from timeline data.
type SeasonalityPoint struct {
	Period string  `json:"period"` // e.g. 2026-05
	Avg    float64 `json:"avg"`
	Max    float64 `json:"max"`
	N      int     `json:"n"`
}

// Opportunities ranks terms from a SINGLE source only.
//
// Scoring rules (never mixed across engines):
//   source=related (default): score = extracted_value from related_queries.rising
//     (Google's rising scale: relative % increase; "Breakout" often appears as a high number e.g. 5000).
//   source=trending: score = increase_percentage from trending_searches
//     (percent increase over the trending window; different unit from related rising).
//   source=all: returns related results first (tagged related_rising), then trending
//     (tagged trending), each ranked within its own unit — scores are NOT comparable across groups.
//
// Snapshots used: latest RELATED_QUERIES snapshots matching seed (for related);
// trending_now snapshots whose response is FTS-matched to seed (for trending).
func Opportunities(s *store.Store, seed string, limit int, source string) ([]Opportunity, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	seed = strings.TrimSpace(strings.ToLower(seed))
	if seed == "" {
		return nil, fmt.Errorf("seed required")
	}
	if limit <= 0 {
		limit = 20
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "related"
	}
	if source != "related" && source != "trending" && source != "all" {
		return nil, fmt.Errorf("source must be related|trending|all")
	}

	hits, err := s.SearchFTS(seed, 50)
	if err != nil {
		return nil, err
	}

	var relatedOut, trendingOut []Opportunity

	if source == "related" || source == "all" {
		seen := map[string]bool{}
		for _, sn := range hits {
			if sn.Engine != "google_trends" {
				continue
			}
			if !strings.Contains(sn.ParamsJSON, "RELATED_QUERIES") {
				continue
			}
			var resp map[string]interface{}
			if err := json.Unmarshal([]byte(sn.ResponseJSON), &resp); err != nil {
				continue
			}
			rq, _ := resp["related_queries"].(map[string]interface{})
			if rq == nil {
				continue
			}
			// rising only — top is absolute popularity, not increase
			arr, _ := rq["rising"].([]interface{})
			for _, item := range arr {
				m, _ := item.(map[string]interface{})
				if m == nil {
					continue
				}
				q, _ := m["query"].(string)
				if q == "" || seen[q] {
					continue
				}
				seen[q] = true
				ev := toFloat(m["extracted_value"])
				relatedOut = append(relatedOut, Opportunity{
					Query:              q,
					IncreasePercentage: ev,
					Source:             "related_rising",
					ParentQ:            seed,
					Score:              ev,
				})
			}
		}
		sort.Slice(relatedOut, func(i, j int) bool { return relatedOut[i].Score > relatedOut[j].Score })
	}

	if source == "trending" || source == "all" {
		seen := map[string]bool{}
		trendSnaps, err := s.ListByEngine("google_trends_trending_now", 10)
		if err != nil {
			return nil, err
		}
		for _, sn := range trendSnaps {
			var resp map[string]interface{}
			if err := json.Unmarshal([]byte(sn.ResponseJSON), &resp); err != nil {
				continue
			}
			arr, _ := resp["trending_searches"].([]interface{})
			for _, item := range arr {
				m, _ := item.(map[string]interface{})
				if m == nil {
					continue
				}
				q, _ := m["query"].(string)
				if q == "" || seen[q] {
					continue
				}
				ql := strings.ToLower(q)
				inc := toFloat(m["increase_percentage"])
				if seed != "" && !strings.Contains(ql, seed) && inc < 500 {
					continue
				}
				seen[q] = true
				vol := toFloat(m["search_volume"])
				trendingOut = append(trendingOut, Opportunity{
					Query:              q,
					IncreasePercentage: inc,
					SearchVolume:       vol,
					Source:             "trending",
					Score:              inc,
				})
			}
		}
		sort.Slice(trendingOut, func(i, j int) bool { return trendingOut[i].Score > trendingOut[j].Score })
	}

	var out []Opportunity
	switch source {
	case "related":
		out = relatedOut
	case "trending":
		out = trendingOut
	default:
		out = append(out, relatedOut...)
		out = append(out, trendingOut...)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Changes diffs related-query sets across the two most recent RELATED_QUERIES snapshots for parentQ.
func Changes(s *store.Store, parentQ string) (*ChangesDiff, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	parentQ = strings.TrimSpace(parentQ)
	if parentQ == "" {
		return nil, fmt.Errorf("parent q required")
	}

	hits, err := s.SearchFTS(`"`+parentQ+`"`, 30)
	if err != nil {
		return nil, err
	}
	var related []store.Snapshot
	for _, sn := range hits {
		if sn.Engine != "google_trends" {
			continue
		}
		if !strings.Contains(sn.ParamsJSON, "RELATED_QUERIES") && !strings.Contains(sn.ParamsJSON, "RELATED_TOPICS") {
			continue
		}
		if !strings.Contains(strings.ToLower(sn.ParamsJSON), strings.ToLower(parentQ)) {
			continue
		}
		related = append(related, sn)
	}
	sort.Slice(related, func(i, j int) bool {
		if related[i].ID != related[j].ID {
			return related[i].ID > related[j].ID
		}
		return related[i].CreatedAt.After(related[j].CreatedAt)
	})
	if len(related) < 2 {
		diff := &ChangesDiff{ParentQ: parentQ, Added: []string{}, Removed: []string{}, Unchanged: []string{}}
		if len(related) == 1 {
			diff.ToID = related[0].ID
			diff.Unchanged = extractRelatedQueries(related[0].ResponseJSON)
		}
		return diff, nil
	}
	newer, older := related[0], related[1]
	newSet := toSet(extractRelatedQueries(newer.ResponseJSON))
	oldSet := toSet(extractRelatedQueries(older.ResponseJSON))

	diff := &ChangesDiff{ParentQ: parentQ, FromID: older.ID, ToID: newer.ID}
	for q := range newSet {
		if oldSet[q] {
			diff.Unchanged = append(diff.Unchanged, q)
		} else {
			diff.Added = append(diff.Added, q)
		}
	}
	for q := range oldSet {
		if !newSet[q] {
			diff.Removed = append(diff.Removed, q)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Unchanged)
	return diff, nil
}
