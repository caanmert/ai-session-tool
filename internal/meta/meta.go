// Package meta stores your own annotations of sessions: a custom title,
// tags, pinned and archived flags. They live in ais's data directory, keyed
// by tool and session id, and never touch the tools' own files.
package meta

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// DefaultDir is $AIS_DATA_DIR, else ~/Library/Application Support/ais on
// macOS, else $XDG_DATA_HOME/ais or ~/.local/share/ais.
func DefaultDir() (string, error) {
	if d := os.Getenv("AIS_DATA_DIR"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "darwin" {
		d, err := os.UserConfigDir() // ~/Library/Application Support
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "ais"), nil
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "ais"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "ais"), nil
}

// Annotation is what you added to one session.
type Annotation struct {
	Title    string
	Tags     []string
	Pinned   bool
	Archived bool
}

func (a Annotation) empty() bool {
	return a.Title == "" && len(a.Tags) == 0 && !a.Pinned && !a.Archived
}

// Store is the annotation database.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (creating if needed) the store at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS annotations (
			tool     TEXT NOT NULL,
			id       TEXT NOT NULL,
			title    TEXT NOT NULL DEFAULT '',
			pinned   INTEGER NOT NULL DEFAULT 0,
			archived INTEGER NOT NULL DEFAULT 0,
			updated  INTEGER NOT NULL,
			PRIMARY KEY (tool, id)
		);
		CREATE TABLE IF NOT EXISTS tags (
			tool TEXT NOT NULL,
			id   TEXT NOT NULL,
			tag  TEXT NOT NULL,
			PRIMARY KEY (tool, id, tag)
		);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("annotations %s: %w", path, err)
	}
	return &Store{db: db, path: path}, nil
}

// Path is where the store lives.
func (st *Store) Path() string { return st.path }

// Close closes the database.
func (st *Store) Close() error { return st.db.Close() }

// All returns every annotation keyed "tool/id".
func (st *Store) All(ctx context.Context) (map[string]Annotation, error) {
	out := map[string]Annotation{}
	rows, err := st.db.QueryContext(ctx, `SELECT tool, id, title, pinned, archived FROM annotations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tool, id string
		var a Annotation
		if err := rows.Scan(&tool, &id, &a.Title, &a.Pinned, &a.Archived); err != nil {
			return nil, err
		}
		out[tool+"/"+id] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	trows, err := st.db.QueryContext(ctx, `SELECT tool, id, tag FROM tags ORDER BY tag`)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var tool, id, tag string
		if err := trows.Scan(&tool, &id, &tag); err != nil {
			return nil, err
		}
		a := out[tool+"/"+id]
		a.Tags = append(a.Tags, tag)
		out[tool+"/"+id] = a
	}
	return out, trows.Err()
}

// update applies fn to one session's annotation row inside a transaction.
func (st *Store) update(ctx context.Context, s model.Session, set string, arg any) error {
	_, err := st.db.ExecContext(ctx, `
		INSERT INTO annotations(tool, id, updated) VALUES (?, ?, ?)
		ON CONFLICT(tool, id) DO NOTHING`, string(s.Tool), s.ID, time.Now().UnixNano())
	if err != nil {
		return err
	}
	_, err = st.db.ExecContext(ctx, `UPDATE annotations SET `+set+` = ?, updated = ? WHERE tool = ? AND id = ?`,
		arg, time.Now().UnixNano(), string(s.Tool), s.ID)
	return err
}

// SetTitle renames a session in ais; "" restores the tool's title.
func (st *Store) SetTitle(ctx context.Context, s model.Session, title string) error {
	return st.update(ctx, s, "title", strings.TrimSpace(title))
}

// SetPinned pins or unpins a session.
func (st *Store) SetPinned(ctx context.Context, s model.Session, pinned bool) error {
	return st.update(ctx, s, "pinned", pinned)
}

// SetArchived hides or shows a session in default lists.
func (st *Store) SetArchived(ctx context.Context, s model.Session, archived bool) error {
	return st.update(ctx, s, "archived", archived)
}

// Tag adds and removes tags. Tags are normalized (see NormalizeTag).
func (st *Store) Tag(ctx context.Context, s model.Session, add, remove []string) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, raw := range add {
		tag, err := NormalizeTag(raw)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(tool, id, tag) VALUES (?, ?, ?)`, string(s.Tool), s.ID, tag); err != nil {
			return err
		}
	}
	for _, raw := range remove {
		tag, err := NormalizeTag(raw)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE tool = ? AND id = ? AND tag = ?`, string(s.Tool), s.ID, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TagCounts returns how many sessions carry each tag.
func (st *Store) TagCounts(ctx context.Context) (map[string]int, error) {
	rows, err := st.db.QueryContext(ctx, `SELECT tag, COUNT(*) FROM tags GROUP BY tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var tag string
		var n int
		if err := rows.Scan(&tag, &n); err != nil {
			return nil, err
		}
		out[tag] = n
	}
	return out, rows.Err()
}

// NormalizeTag lowercases a tag and strips a leading "#". Tags may use
// letters, digits and the symbols - _ . / :, up to 40 characters.
func NormalizeTag(raw string) (string, error) {
	tag := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
	if tag == "" || len([]rune(tag)) > 40 {
		return "", fmt.Errorf("invalid tag %q: use 1-40 letters, digits and the symbols - _ . / : only", raw)
	}
	for _, r := range tag {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:", r) || r > 127
		if !ok {
			return "", fmt.Errorf("invalid tag %q: use letters, digits and the symbols - _ . / : only", raw)
		}
	}
	return tag, nil
}

// Apply copies annotations onto sessions and sorts them: pinned first,
// then most recently updated.
func Apply(sessions []model.Session, ann map[string]Annotation) {
	for i := range sessions {
		s := &sessions[i]
		a, ok := ann[s.Key()]
		if !ok || a.empty() {
			continue
		}
		if a.Title != "" && a.Title != s.Title {
			s.OriginalTitle, s.Title = s.Title, a.Title
		}
		s.Tags = append([]string(nil), a.Tags...)
		s.Pinned, s.Archived = a.Pinned, a.Archived
	}
	Sort(sessions)
}

// Sort orders sessions pinned first, then newest first.
func Sort(sessions []model.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		a, b := sessions[i], sessions[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})
}
