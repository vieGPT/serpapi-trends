---
name: serpapi-trends
description: Run and interpret the SerpAPI Google Trends CLI (serpapi-trends) for US-scoped interest, related queries, trending now, autocomplete, and local SQLite compounds. Use when the user asks for Google Trends, SerpAPI trends, rising niche signals, opportunities, changes, geo-gap, seasonality, or local trend portfolio. Also use when building or fixing this CLI.
---

# serpapi-trends

## Overview

CLI at `github.com/vieGPT/serpapi-trends`. Three SerpAPI engines, local SQLite snapshots, pure-logic compounds. Not video. Not an MCP server.

## Install and auth

```bash
git clone https://github.com/vieGPT/serpapi-trends.git
cd serpapi-trends
go build -o serpapi-trends ./cmd/serpapi-trends
export SERPAPI_API_KEY=...   # or SERPAPI_KEY
```

- Key runtime-only. Never write it to git, artifacts, logs, or memory.
- `doctor` without key must exit non-zero.

## Defaults

- `geo=US` on interest, related, region, trending
- `hl=en` global
- Override with flags. Age filters do not exist on this API.

## Commands

- Live (auto-save snapshots) — `doctor`, `account`, `autocomplete`, `trending`, `interest`, `related`, `region`
- Local — `history search`, `stale`, `opportunities`, `changes`, `geo-gap`, `seasonality`

### opportunities scoring

- `--source related` (default) — related_queries.rising extracted_value
- `--source trending` — increase_percentage
- Never mix units in one ranked list

### seasonality

Meaningful only on long TIMESERIES (e.g. `today 5-y`), not short windows.

## Agent rules

1. One Trends pull is a hypothesis, not a niche decision.
2. Report **as-of date** with every rising/cooling claim.
3. Prefer stable `SERPAPI_TRENDS_HOME` (not `/tmp`) for real research.
4. For YouTube, combine with Printing Press youtube-pp when the user asks — this tool does not replace it.
5. Data order Max locked — performance data first (views, Trends), decide angle, then Quora for language. Ads later.
6. Do not invent SerpAPI parameters. If unsure, stop and check the contract or docs.

## Data path

`$SERPAPI_TRENDS_HOME/snapshots.db` or `~/.local/share/serpapi-trends/snapshots.db`.

## References

- Repo README and AGENTS.md
- Optional deeper notes under this skill `references/` when added
