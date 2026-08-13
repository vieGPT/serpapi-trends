# serpapi-trends

SerpAPI Google Trends CLI — local portfolio for YouTube niche speed-to-market.

Engines: `google_trends`, `google_trends_trending_now`, `google_trends_autocomplete`.

## Install

### From this tree

```bash
cd /path/to/serpapi-trends
go build -o serpapi-trends ./cmd/serpapi-trends
# or
go install ./cmd/serpapi-trends
```

Requires Go 1.22+.

### Auth

```bash
export SERPAPI_API_KEY=your_key   # or SERPAPI_KEY
```

Never pass the key on the command line. `doctor` fingerprints only.

### Data dir

Snapshots land under `$SERPAPI_TRENDS_HOME/snapshots.db` or  
`~/.local/share/serpapi-trends/snapshots.db`.

## Commands

```
serpapi-trends doctor
serpapi-trends account
serpapi-trends autocomplete <q>
serpapi-trends trending [--geo US] [--hours 24] [--only-active]
serpapi-trends interest --q <q> [--date today 3-m] [--data-type TIMESERIES]
serpapi-trends related --q <q> [--data-type RELATED_QUERIES] [--date today 12-m]
serpapi-trends region --q <q> [--data-type GEO_MAP_0]
serpapi-trends history search <term>
serpapi-trends stale --older-than 168h
serpapi-trends opportunities <seed> [--source related|trending|all]
serpapi-trends changes <q>
serpapi-trends geo-gap <seed>
serpapi-trends seasonality <q>
```

All support `--json`.

## Notes

- Successful live calls auto-save to the local SQLite portfolio.
- `opportunities --source related` ranks on related_queries.rising extracted_value only.
- `opportunities --source trending` ranks on increase_percentage only. Units are never mixed.
- `seasonality` is meaningful on multi-year TIMESERIES (e.g. `--date today 5-y` when collecting).
