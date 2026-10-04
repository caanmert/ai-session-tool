// Package index caches parsed sessions in SQLite so ais starts instantly,
// and indexes conversation text for full-text search.
//
// The index is only a cache: deleting it loses nothing. Sync re-parses a
// transcript only when its size or modification time changed, when the
// provider's fingerprint changed, or when schemaVersion is bumped.
package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // pure-Go SQLite driver with FTS5

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

// schemaVersion invalidates the whole index when the schema or what the
// parsers extract changes.
const schemaVersion = 1

// maxBodyBytes caps the conversation text indexed per session.
const maxBodyBytes = 1 << 20

// DefaultPath is $AIS_CACHE_DIR/index.db, else the OS cache directory
// (~/Library/Caches/ais on macOS, ~/.cache/ais on Linux).
func DefaultPath() (string, error) {
	if d := os.Getenv("AIS_CACHE_DIR"); d != "" {
		return filepath.Join(d, "index.db"), nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "ais", "index.db"), nil
}

// Index is an open session index.
type Index struct {
	db   *sql.DB
	path string

	mu                  sync.Mutex
	lastParsed, lastDel int
}

// LastSync reports how many transcripts the last Sync parsed and how many
// it dropped because they disappeared.
func (ix *Index) LastSync() (parsed, removed int) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.lastParsed, ix.lastDel
}

// Open opens (creating if needed) the index at path, rebuilding it when it
// was written by a different schema version.
func Open(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	ix := &Index{db: db, path: path}
	if err := ix.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("index %s: %w", path, err)
	}
	return ix, nil
}

// Path is where the index lives.
func (ix *Index) Path() string { return ix.path }

// Close closes the database.
func (ix *Index) Close() error { return ix.db.Close() }

func (ix *Index) migrate() error {
	var version int
	if err := ix.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version == schemaVersion {
		return nil
	}
	stmts := []string{
		`DROP TABLE IF EXISTS fts`,
		`DROP TABLE IF EXISTS warnings`,
		`DROP TABLE IF EXISTS sessions`,
		`DROP TABLE IF EXISTS files`,
		`DROP TABLE IF EXISTS meta`,
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE files (
			path   TEXT PRIMARY KEY,
			tool   TEXT NOT NULL,
			size   INTEGER NOT NULL,
			mtime  INTEGER NOT NULL,
			status TEXT NOT NULL, -- ok, empty, hidden, error
			error  TEXT
		)`,
		`CREATE TABLE sessions (
			rowid   INTEGER PRIMARY KEY,
			path    TEXT NOT NULL UNIQUE REFERENCES files(path) ON DELETE CASCADE,
			tool    TEXT NOT NULL,
			id      TEXT NOT NULL,
			updated INTEGER NOT NULL,
			data    TEXT NOT NULL -- model.Session as JSON
		)`,
		`CREATE INDEX sessions_updated ON sessions(updated DESC)`,
		`CREATE TABLE warnings (
			file TEXT NOT NULL REFERENCES files(path) ON DELETE CASCADE,
			path TEXT NOT NULL,
			line INTEGER NOT NULL,
			msg  TEXT NOT NULL
		)`,
		`CREATE INDEX warnings_file ON warnings(file)`,
		// rowid = sessions.rowid
		`CREATE VIRTUAL TABLE fts USING fts5(meta, body, tokenize = 'unicode61 remove_diacritics 2')`,
		fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion),
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("%s: %w", strings.Fields(s)[0], err)
		}
	}
	return tx.Commit()
}

// Load returns the cached sessions without touching any transcript, with
// live state attached. It is fast enough to paint a UI before Sync.
func (ix *Index) Load(ctx context.Context, providers []provider.Provider) (provider.ScanResult, error) {
	var res provider.ScanResult
	tools := toolSet(providers)

	rows, err := ix.db.QueryContext(ctx, `SELECT tool, data FROM sessions ORDER BY updated DESC, id`)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		var tool, data string
		if err := rows.Scan(&tool, &data); err != nil {
			return res, err
		}
		if !tools[tool] {
			continue
		}
		var s model.Session
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			continue
		}
		res.Sessions = append(res.Sessions, s)
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	wrows, err := ix.db.QueryContext(ctx, `SELECT w.path, w.line, w.msg, f.tool FROM warnings w JOIN files f ON f.path = w.file ORDER BY w.path, w.line`)
	if err != nil {
		return res, err
	}
	defer wrows.Close()
	for wrows.Next() {
		var w provider.Warning
		var tool string
		if err := wrows.Scan(&w.Path, &w.Line, &w.Msg, &tool); err != nil {
			return res, err
		}
		if tools[tool] {
			res.Warnings = append(res.Warnings, w)
		}
	}

	srows, err := ix.db.QueryContext(ctx, `SELECT tool, status, COUNT(*) FROM files WHERE status IN ('empty', 'hidden') GROUP BY tool, status`)
	if err != nil {
		return res, err
	}
	defer srows.Close()
	for srows.Next() {
		var tool, status string
		var n int
		if err := srows.Scan(&tool, &status, &n); err != nil {
			return res, err
		}
		if !tools[tool] {
			continue
		}
		if status == "empty" {
			res.Empty += n
		} else {
			res.Hidden += n
		}
	}

	provider.AttachLive(ctx, providers, res.Sessions)
	provider.SortByUpdated(res.Sessions)
	return res, nil
}

func toolSet(providers []provider.Provider) map[string]bool {
	m := map[string]bool{}
	for _, p := range providers {
		m[string(p.Tool())] = true
	}
	return m
}

// parsed is the outcome of parsing one changed transcript.
type parsed struct {
	tool    model.Tool
	ref     provider.FileRef
	status  string
	err     string
	session model.Session
	body    string
	warns   []provider.Warning
}

// Sync brings the index up to date with the transcripts on disk, then
// returns every session like Load.
func (ix *Index) Sync(ctx context.Context, providers []provider.Provider) (provider.ScanResult, error) {
	type job struct {
		p   provider.Provider
		ref provider.FileRef
	}
	var (
		jobs    []job
		removed []string
		resetFP = map[string]string{} // tool -> new fingerprint
	)
	for _, p := range providers {
		tool := string(p.Tool())
		fp := ""
		if f, ok := p.(provider.Fingerprinter); ok {
			fp = f.Fingerprint()
		}
		var stored string
		err := ix.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, "fingerprint:"+tool).Scan(&stored)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return provider.ScanResult{}, err
		}
		stale := err == nil && stored != fp || errors.Is(err, sql.ErrNoRows) && fp != ""
		if stale || errors.Is(err, sql.ErrNoRows) {
			resetFP[tool] = fp
		}

		refs, err := p.Discover(ctx)
		if err != nil {
			return provider.ScanResult{}, fmt.Errorf("%s: discover: %w", tool, err)
		}
		known, err := ix.files(ctx, tool)
		if err != nil {
			return provider.ScanResult{}, err
		}
		seen := map[string]bool{}
		for _, ref := range refs {
			seen[ref.Path] = true
			k, ok := known[ref.Path]
			if stale || !ok || k.size != ref.Size || k.mtime != ref.ModTime.UnixNano() {
				jobs = append(jobs, job{p, ref})
			}
		}
		for path := range known {
			if !seen[path] {
				removed = append(removed, path)
			}
		}
	}

	results := make([]parsed, len(jobs))
	var wg sync.WaitGroup
	next := make(chan int)
	for range min(runtime.NumCPU(), max(len(jobs), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				results[i] = parse(ctx, jobs[i].p, jobs[i].ref)
			}
		}()
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return provider.ScanResult{}, err
	}

	if len(results) > 0 || len(removed) > 0 || len(resetFP) > 0 {
		if err := ix.write(ctx, results, removed, resetFP); err != nil {
			return provider.ScanResult{}, err
		}
	}
	ix.mu.Lock()
	ix.lastParsed, ix.lastDel = len(results), len(removed)
	ix.mu.Unlock()
	return ix.Load(ctx, providers)
}

type fileState struct{ size, mtime int64 }

func (ix *Index) files(ctx context.Context, tool string) (map[string]fileState, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT path, size, mtime FROM files WHERE tool = ?`, tool)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]fileState{}
	for rows.Next() {
		var path string
		var st fileState
		if err := rows.Scan(&path, &st.size, &st.mtime); err != nil {
			return nil, err
		}
		m[path] = st
	}
	return m, rows.Err()
}

func parse(ctx context.Context, p provider.Provider, ref provider.FileRef) parsed {
	r := parsed{tool: p.Tool(), ref: ref}
	s, warns, err := p.Parse(ctx, ref)
	r.warns = warns
	switch {
	case errors.Is(err, provider.ErrEmpty):
		r.status = "empty"
		return r
	case errors.Is(err, provider.ErrHidden):
		r.status = "hidden"
		return r
	case err != nil:
		r.status, r.err = "error", err.Error()
		r.warns = append(r.warns, provider.Warning{Path: ref.Path, Msg: err.Error()})
		return r
	}
	s.Live = nil
	r.status, r.session = "ok", s
	if msgs, err := p.Transcript(ctx, s); err == nil {
		r.body = body(msgs)
	}
	return r
}

// body is the searchable text of a conversation: prompts, replies and tool
// calls (not tool output, which is mostly noise).
func body(msgs []model.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if b.Len() >= maxBodyBytes {
			break
		}
		switch m.Kind {
		case model.KindText:
			b.WriteString(m.Text)
		case model.KindToolUse:
			b.WriteString(m.ToolName + " " + m.Text)
		default:
			continue
		}
		b.WriteString("\n")
	}
	return b.String()
}

// meta is the searchable metadata of a session.
func meta(s model.Session) string {
	return strings.Join([]string{s.Title, s.Summary, s.FirstPrompt, s.CWD, s.GitBranch, s.Model, s.ID, string(s.Tool)}, "\n")
}

func (ix *Index) write(ctx context.Context, results []parsed, removed []string, fingerprints map[string]string) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	drop := func(path string) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fts WHERE rowid IN (SELECT rowid FROM sessions WHERE path = ?)`, path); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM files WHERE path = ?`, path) // cascades
		return err
	}
	for _, path := range removed {
		if err := drop(path); err != nil {
			return err
		}
	}
	for _, r := range results {
		if err := drop(r.ref.Path); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO files(path, tool, size, mtime, status, error) VALUES (?, ?, ?, ?, ?, ?)`,
			r.ref.Path, string(r.tool), r.ref.Size, r.ref.ModTime.UnixNano(), r.status, r.err); err != nil {
			return err
		}
		for _, w := range r.warns {
			if _, err := tx.ExecContext(ctx, `INSERT INTO warnings(file, path, line, msg) VALUES (?, ?, ?, ?)`, r.ref.Path, w.Path, w.Line, w.Msg); err != nil {
				return err
			}
		}
		if r.status != "ok" {
			continue
		}
		data, err := json.Marshal(r.session)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO sessions(path, tool, id, updated, data) VALUES (?, ?, ?, ?, ?)`,
			r.ref.Path, string(r.tool), r.session.ID, r.session.UpdatedAt.UnixNano(), string(data))
		if err != nil {
			return err
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO fts(rowid, meta, body) VALUES (?, ?, ?)`, rowid, meta(r.session), r.body); err != nil {
			return err
		}
	}
	for tool, fp := range fingerprints {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, "fingerprint:"+tool, fp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Stats describes the index for `ais doctor`.
type Stats struct {
	Files, Sessions int
	Bytes           int64
}

// Stats counts what the index holds.
func (ix *Index) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	if err := ix.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM files), (SELECT COUNT(*) FROM sessions)`).Scan(&st.Files, &st.Sessions); err != nil {
		return st, err
	}
	for _, suffix := range []string{"", "-wal"} {
		if info, err := os.Stat(ix.path + suffix); err == nil {
			st.Bytes += info.Size()
		}
	}
	return st, nil
}
