package logic

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/vieGPT/serpapi-trends/internal/store"
)

type Opportunity struct {
	Query              string  `json:"query"`
	IncreasePercentage float64 `json:"increase_percentage,omitempty"`
	SearchVolume       float64 `json:"search_volume,omitempty"`
	Source             string  `json:"source"`
	ParentQ            string  `json:"parent_q,omitempty"`
	Score              float64 `json:"score"`
}

type ChangesDiff struct {
	ParentQ   string   `json:"parent_q"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Unchanged []string `json:"unchanged"`
	FromID    int64    `json:"from_id,omitempty"`
	ToID      int64    `json:"to_id,omitempty"`
}

type GeoGapEntry struct {
	Geo            string  `json:"geo"`
	Location       string  `json:"location,omitempty"`
	ExtractedValue float64 `json:"extracted_value"`
	Query          string  `json:"query,omitempty"`
}

type SeasonalityPoint struct {
	Period string  `json:"period"`
	Avg    float64 `json:"avg"`
	Max    float64 `json:"max"`
	N      int     `json:"n"`
}

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
			if sn.Engine != "google_trends" || !strings.Contains(sn.ParamsJSON, "RELATED_QUERIES") {
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
				relatedOut = append(relatedOut, Opportunity{Query: q, IncreasePercentage: ev, Source: "related_rising", ParentQ: seed, Score: ev})
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
				trendingOut = append(trendingOut, Opportunity{Query: q, IncreasePercentage: inc, SearchVolume: vol, Source: "trending", Score: inc})
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
		out = append(append(out, relatedOut...), trendingOut...)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

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

func GeoGap(s *store.Store, seed string, limit int) ([]GeoGapEntry, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if limit <= 0 {
		limit = 20
	}
	hits, err := s.SearchFTS(seed, 30)
	if err != nil {
		return nil, err
	}
	out := make([]GeoGapEntry, 0)
	for _, sn := range hits {
		if sn.Engine != "google_trends" {
			continue
		}
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(sn.ResponseJSON), &resp); err != nil {
			continue
		}
		if arr, ok := resp["interest_by_region"].([]interface{}); ok {
			for _, item := range arr {
				m, _ := item.(map[string]interface{})
				if m == nil {
					continue
				}
				out = append(out, GeoGapEntry{Geo: str(m["geo"]), Location: str(m["location"]), ExtractedValue: toFloat(m["extracted_value"]), Query: seed})
			}
		}
		if arr, ok := resp["compared_breakdown_by_region"].([]interface{}); ok {
			for _, item := range arr {
				m, _ := item.(map[string]interface{})
				if m == nil {
					continue
				}
				vals, _ := m["values"].([]interface{})
				var maxEv float64
				var maxQ string
				for _, v := range vals {
					vm, _ := v.(map[string]interface{})
					if vm == nil {
						continue
					}
					ev := toFloat(vm["extracted_value"])
					if ev >= maxEv {
						maxEv = ev
						maxQ = str(vm["query"])
					}
				}
				out = append(out, GeoGapEntry{Geo: str(m["geo"]), Location: str(m["location"]), ExtractedValue: maxEv, Query: maxQ})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExtractedValue > out[j].ExtractedValue })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func Seasonality(s *store.Store, seed string) ([]SeasonalityPoint, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	hits, err := s.SearchFTS(seed, 20)
	if err != nil {
		return nil, err
	}
	type acc struct {
		sum, max float64
		n        int
	}
	byPeriod := map[string]*acc{}
	for _, sn := range hits {
		if sn.Engine != "google_trends" || !strings.Contains(sn.ParamsJSON, "TIMESERIES") {
			continue
		}
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(sn.ResponseJSON), &resp); err != nil {
			continue
		}
		iot, _ := resp["interest_over_time"].(map[string]interface{})
		if iot == nil {
			continue
		}
		timeline, _ := iot["timeline_data"].([]interface{})
		for _, pt := range timeline {
			m, _ := pt.(map[string]interface{})
			if m == nil {
				continue
			}
			period := monthKey(str(m["date"]))
			if period == "" {
				continue
			}
			var ev float64
			if vals, ok := m["values"].([]interface{}); ok && len(vals) > 0 {
				vm, _ := vals[0].(map[string]interface{})
				ev = toFloat(vm["extracted_value"])
			} else {
				ev = toFloat(m["extracted_value"])
			}
			a := byPeriod[period]
			if a == nil {
				a = &acc{}
				byPeriod[period] = a
			}
			a.sum += ev
			a.n++
			if ev > a.max {
				a.max = ev
			}
		}
	}
	periods := make([]string, 0, len(byPeriod))
	for p := range byPeriod {
		periods = append(periods, p)
	}
	sort.Strings(periods)
	out := make([]SeasonalityPoint, 0, len(periods))
	for _, p := range periods {
		a := byPeriod[p]
		avg := 0.0
		if a.n > 0 {
			avg = a.sum / float64(a.n)
		}
		out = append(out, SeasonalityPoint{Period: p, Avg: avg, Max: a.max, N: a.n})
	}
	return out, nil
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		s := strings.TrimSpace(x)
		s = strings.TrimSuffix(s, "+")
		s = strings.ReplaceAll(s, ",", "")
		if strings.HasSuffix(strings.ToUpper(s), "K") {
			f, _ := strconv.ParseFloat(s[:len(s)-1], 64)
			return f * 1000
		}
		f, _ := strconv.ParseFloat(s, 64)
		return f
	default:
		return 0
	}
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func extractRelatedQueries(responseJSON string) []string {
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(responseJSON), &resp); err != nil {
		return nil
	}
	rq, _ := resp["related_queries"].(map[string]interface{})
	if rq == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, bucket := range []string{"rising", "top"} {
		arr, _ := rq[bucket].([]interface{})
		for _, item := range arr {
			m, _ := item.(map[string]interface{})
			if m == nil {
				continue
			}
			q := str(m["query"])
			if q != "" && !seen[q] {
				seen[q] = true
				out = append(out, q)
			}
		}
	}
	return out
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func monthKey(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)
	if len(dateStr) >= 7 && dateStr[4] == '-' {
		return dateStr[:7]
	}
	months := map[string]string{
		"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04",
		"May": "05", "Jun": "06", "Jul": "07", "Aug": "08",
		"Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12",
	}
	parts := strings.Fields(dateStr)
	if len(parts) < 2 {
		return ""
	}
	mon := parts[0][:3]
	year := strings.Trim(parts[len(parts)-1], ",")
	if mm, ok := months[mon]; ok && len(year) == 4 {
		return year + "-" + mm
	}
	return ""
}
