package logic

import (
	"testing"
)

func TestOpportunities_table(t *testing.T) {
	s := openTempStore(t)

	// synthetic related_queries snapshot
	relatedResp := map[string]interface{}{
		"related_queries": map[string]interface{}{
			"rising": []interface{}{
				map[string]interface{}{"query": "enoch book pdf", "extracted_value": 350.0},
				map[string]interface{}{"query": "nephilim", "extracted_value": 120.0},
			},
			"top": []interface{}{
				map[string]interface{}{"query": "book of enoch", "extracted_value": 100.0},
			},
		},
	}
	if err := s.Save("google_trends", map[string]string{"q": "book of enoch", "data_type": "RELATED_QUERIES"}, relatedResp); err != nil {
		t.Fatal(err)
	}

	trendingResp := map[string]interface{}{
		"trending_searches": []interface{}{
			map[string]interface{}{"query": "enoch prophecy", "increase_percentage": 500.0, "search_volume": 20000.0},
			map[string]interface{}{"query": "unrelated sports", "increase_percentage": 50.0, "search_volume": 1000.0},
		},
	}
	if err := s.Save("google_trends_trending_now", map[string]string{"geo": "US", "hours": "24"}, trendingResp); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		seed      string
		wantMin   int
		wantErr   bool
		wantQuery string // at least one result contains this
	}{
		{"empty seed", "", 0, true, ""},
		{"enoch seed", "enoch", 1, false, "enoch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Opportunities(s, tt.seed, 10, "related")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) < tt.wantMin {
				t.Fatalf("len=%d want >= %d", len(got), tt.wantMin)
			}
			if tt.wantQuery != "" {
				found := false
				for _, o := range got {
					if containsFold(o.Query, tt.wantQuery) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("no result containing %q in %+v", tt.wantQuery, got)
				}
			}
		})
	}
}

func TestChanges_table(t *testing.T) {
	s := openTempStore(t)
	oldResp := map[string]interface{}{
		"related_queries": map[string]interface{}{
			"rising": []interface{}{
				map[string]interface{}{"query": "alpha"},
				map[string]interface{}{"query": "beta"},
			},
		},
	}
	newResp := map[string]interface{}{
		"related_queries": map[string]interface{}{
			"rising": []interface{}{
				map[string]interface{}{"query": "beta"},
				map[string]interface{}{"query": "gamma"},
			},
		},
	}
	_ = s.Save("google_trends", map[string]string{"q": "seed", "data_type": "RELATED_QUERIES"}, oldResp)
	_ = s.Save("google_trends", map[string]string{"q": "seed", "data_type": "RELATED_QUERIES"}, newResp)

	diff, err := Changes(s, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if diff.ParentQ != "seed" {
		t.Fatalf("parent=%s", diff.ParentQ)
	}
	// With two snapshots we expect added gamma, removed alpha, unchanged beta
	if !containsStr(diff.Added, "gamma") {
		t.Fatalf("added=%v", diff.Added)
	}
	if !containsStr(diff.Removed, "alpha") {
		t.Fatalf("removed=%v", diff.Removed)
	}
	if !containsStr(diff.Unchanged, "beta") {
		t.Fatalf("unchanged=%v", diff.Unchanged)
	}
}

func TestGeoGap_emptyOK(t *testing.T) {
	s := openTempStore(t)
	got, err := GeoGap(s, "enoch", 5)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil slice")
	}
}

func TestSeasonality_table(t *testing.T) {
	s := openTempStore(t)
	resp := map[string]interface{}{
		"interest_over_time": map[string]interface{}{
			"timeline_data": []interface{}{
				map[string]interface{}{
					"date": "May 13, 2026",
					"values": []interface{}{
						map[string]interface{}{"extracted_value": 60.0, "query": "book of enoch"},
					},
				},
				map[string]interface{}{
					"date": "May 20, 2026",
					"values": []interface{}{
						map[string]interface{}{"extracted_value": 80.0, "query": "book of enoch"},
					},
				},
				map[string]interface{}{
					"date": "Jun 3, 2026",
					"values": []interface{}{
						map[string]interface{}{"extracted_value": 40.0, "query": "book of enoch"},
					},
				},
			},
		},
	}
	_ = s.Save("google_trends", map[string]string{"q": "book of enoch", "data_type": "TIMESERIES", "date": "today 3-m"}, resp)

	pts, err := Seasonality(s, "enoch")
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) < 2 {
		t.Fatalf("expected at least 2 months, got %v", pts)
	}
	// May avg = (60+80)/2 = 70
	foundMay := false
	for _, p := range pts {
		if p.Period == "2026-05" {
			foundMay = true
			if p.Avg != 70 {
				t.Fatalf("May avg=%v want 70", p.Avg)
			}
			if p.Max != 80 {
				t.Fatalf("May max=%v want 80", p.Max)
			}
		}
	}
	if !foundMay {
		t.Fatalf("missing 2026-05 in %v", pts)
	}
}

func TestMonthKey(t *testing.T) {
	cases := map[string]string{
		"May 13, 2026":     "2026-05",
		"Jun 1 – 7, 2026":  "2026-06",
		"2026-05-13":       "2026-05",
		"":                 "",
	}
	for in, want := range cases {
		if got := monthKey(in); got != want {
			t.Errorf("monthKey(%q)=%q want %q", in, got, want)
		}
	}
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		stringIndexFold(s, sub) >= 0)
}
func stringIndexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	for i := 0; i+lsub <= ls; i++ {
		if equalFold(s[i:i+lsub], sub) {
			return i
		}
	}
	return -1
}
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
func containsStr(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}
