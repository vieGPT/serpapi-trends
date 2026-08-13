# AGENTS.md — serpapi-trends

When a niche starts rising you have days, not weeks.  
`serpapi-trends` puts SerpAPI’s three Google Trends engines (interest over time, Trending Now, autocomplete) into your local data portfolio so you can see volume spikes, interest slopes, and rising related queries before the timeline fills with competitors.

## Auth
```bash
export SERPAPI_API_KEY=your_key   # or SERPAPI_KEY
```
Never pass the key on the CLI. `doctor` only fingerprints the first four characters.

## Core commands (exact paths)
- `serpapi-trends interest` — TIMESERIES / multi-query comparison
- `serpapi-trends related` — RELATED_QUERIES / RELATED_TOPICS
- `serpapi-trends region` — GEO_MAP / GEO_MAP_0
- `serpapi-trends trending` (alias `now`) — google_trends_trending_now
- `serpapi-trends autocomplete` (alias `suggest`)
- `serpapi-trends account` / `doctor`
- `serpapi-trends history search <term>` — local FTS
- `serpapi-trends stale`
- `serpapi-trends opportunities <seed>`
- `serpapi-trends changes <q>`
- `serpapi-trends geo-gap <seed>`
- `serpapi-trends seasonality <q>`

All support `--json` and `--compact`. Live API only when required; history and compounds are local SQLite.

## Why this exists
YouTube and short-form channels lose niches in days. This CLI turns reliable SerpAPI JSON into an offline-first portfolio member that compounds with other sources.
