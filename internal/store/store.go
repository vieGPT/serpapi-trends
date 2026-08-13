// Package store provides an append-only SQLite snapshot mirror for SerpAPI Trends responses.
// Keyed by engine + params hash + timestamp. FTS5-ready for history search.
// Never stores API keys or secrets.
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	engine TEXT NOT NULL,
	params_hash TEXT NOT NULL,
	params_json TEXT NOT NULL,
	response_json TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_engine_hash ON snapshots(engine, params_hash);
CREATE INDEX IF NOT EXISTS idx_snapshots_created ON snapshots(created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS snapshots_fts USING fts5(
	engine,
	params_json,
	response_json,
	content='snapshots',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS snapshots_ai AFTER INSERT ON snapshots BEGIN
	INSERT INTO snapshots_fts(rowid, engine, params_json, response_json)
	VALUES (new.id, new.engine, new.params_json, new.response_json);
END;
`

// Store is the append-only snapshot store.
type Store struct {
	db *sql.DB
}

// DefaultPath returns the default SQLite path under XDG_DATA_HOME or ~/.local/share.
func DefaultPath() string {
	if d := os.Getenv("SERPAPI_TRENDS_HOME"); d != "" {
		return filepath.Join(d, "snapshots.db")
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "serpapi-trends", "snapshots.db")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "serpapi-trends", "snapshots.db")
}

// Open opens or creates the store at path. Creates parent dirs as needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("store open: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying DB.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ParamsHash returns a stable SHA-256 hex of sorted key=value pairs (excluding api_key).
func ParamsHash(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "api_key" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
		b.WriteByte('&')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Snapshot is a stored response.
type Snapshot struct {
	ID           int64
	Engine       string
	ParamsHash   string
	ParamsJSON   string
	ResponseJSON string
	CreatedAt    time.Time
}

// Save appends a snapshot. params must not contain the API key.
func (s *Store) Save(engine string, params map[string]string, response map[string]interface{}) error {
	clean := make(map[string]string, len(params))
	for k, v := range params {
		if k == "api_key" {
			continue
		}
		clean[k] = v
	}
	ph := ParamsHash(clean)
	pj, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	rj, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO snapshots (engine, params_hash, params_json, response_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		engine, ph, string(pj), string(rj), time.Now().Unix(),
	)
	return err
}

// Latest returns the most recent snapshot for engine+params hash, or nil if none.
func (s *Store) Latest(engine string, params map[string]string) (*Snapshot, error) {
	ph := ParamsHash(params)
	row := s.db.QueryRow(
		`SELECT id, engine, params_hash, params_json, response_json, created_at FROM snapshots
		 WHERE engine = ? AND params_hash = ? ORDER BY created_at DESC LIMIT 1`,
		engine, ph,
	)
	var sn Snapshot
	var ts int64
	err := row.Scan(&sn.ID, &sn.Engine, &sn.ParamsHash, &sn.ParamsJSON, &sn.ResponseJSON, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sn.CreatedAt = time.Unix(ts, 0)
	return &sn, nil
}

// Age returns how old the latest matching snapshot is. Returns 0, false if none.
func (s *Store) Age(engine string, params map[string]string) (time.Duration, bool, error) {
	sn, err := s.Latest(engine, params)
	if err != nil || sn == nil {
		return 0, false, err
	}
	return time.Since(sn.CreatedAt), true, nil
}

// SearchFTS runs an FTS5 query over stored snapshots. Returns matching snapshots newest first.
func (s *Store) SearchFTS(query string, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	// Simple sanitize: FTS5 uses double-quote for phrases; escape internal quotes.
	q := strings.ReplaceAll(query, `"`, `""`)
	rows, err := s.db.Query(
		`SELECT s.id, s.engine, s.params_hash, s.params_json, s.response_json, s.created_at
		 FROM snapshots_fts f
		 JOIN snapshots s ON s.id = f.rowid
		 WHERE snapshots_fts MATCH ?
		 ORDER BY s.created_at DESC
		 LIMIT ?`,
		q, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		var ts int64
		if err := rows.Scan(&sn.ID, &sn.Engine, &sn.ParamsHash, &sn.ParamsJSON, &sn.ResponseJSON, &ts); err != nil {
			return nil, err
		}
		sn.CreatedAt = time.Unix(ts, 0)
		out = append(out, sn)
	}
	return out, rows.Err()
}

// ListByEngine returns newest-first snapshots for an engine (capped).
func (s *Store) ListByEngine(engine string, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, engine, params_hash, params_json, response_json, created_at
		FROM snapshots WHERE engine = ? ORDER BY created_at DESC LIMIT ?`, engine, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		var ts int64
		if err := rows.Scan(&sn.ID, &sn.Engine, &sn.ParamsHash, &sn.ParamsJSON, &sn.ResponseJSON, &ts); err != nil {
			return nil, err
		}
		sn.CreatedAt = time.Unix(ts, 0)
		out = append(out, sn)
	}
	return out, rows.Err()
}

// ListStale returns snapshots older than maxAge (by latest per engine+hash).
// For simplicity this returns the latest row per (engine, params_hash) that is older than maxAge.
func (s *Store) ListStale(maxAge time.Duration) ([]Snapshot, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	rows, err := s.db.Query(`
		SELECT id, engine, params_hash, params_json, response_json, created_at FROM (
			SELECT id, engine, params_hash, params_json, response_json, created_at,
			       ROW_NUMBER() OVER (PARTITION BY engine, params_hash ORDER BY created_at DESC) AS rn
			FROM snapshots
		) WHERE rn = 1 AND created_at < ?
		ORDER BY created_at ASC
	`, cutoff)
	if err != nil {
		// Fallback for older SQLite without window functions: simpler query
		rows, err = s.db.Query(`
			SELECT id, engine, params_hash, params_json, response_json, created_at
			FROM snapshots
			WHERE created_at < ?
			ORDER BY created_at ASC
			LIMIT 100
		`, cutoff)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		var ts int64
		if err := rows.Scan(&sn.ID, &sn.Engine, &sn.ParamsHash, &sn.ParamsJSON, &sn.ResponseJSON, &ts); err != nil {
			return nil, err
		}
		sn.CreatedAt = time.Unix(ts, 0)
		out = append(out, sn)
	}
	return out, rows.Err()
}
