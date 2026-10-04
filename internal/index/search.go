package index

import (
	"context"
	"strings"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// Snippet match markers; callers replace them with highlighting.
const (
	MarkStart = "\x02"
	MarkEnd   = "\x03"
)

// Hit is one search result.
type Hit struct {
	Tool    model.Tool
	ID      string
	Snippet string // with MarkStart/MarkEnd around matches; "" when only metadata matched
}

// Key identifies a session across tools.
func Key(tool model.Tool, id string) string { return string(tool) + "/" + id }

// Search finds sessions whose metadata or conversation contains every
// term of q, best matches first. Words match as prefixes ("retr" finds
// "retries"); "double quoted" text matches as a phrase.
func (ix *Index) Search(ctx context.Context, q string, limit int) ([]Hit, error) {
	expr := ftsQuery(q)
	if expr == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 1 << 30
	}
	rows, err := ix.db.QueryContext(ctx, `
		SELECT s.tool, s.id, snippet(fts, 1, ?, ?, '…', 16)
		FROM fts JOIN sessions s ON s.rowid = fts.rowid
		WHERE fts MATCH ?
		ORDER BY bm25(fts, 4.0, 1.0)
		LIMIT ?`, MarkStart, MarkEnd, expr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var tool string
		if err := rows.Scan(&tool, &h.ID, &h.Snippet); err != nil {
			return nil, err
		}
		h.Tool = model.Tool(tool)
		if !strings.Contains(h.Snippet, MarkStart) {
			h.Snippet = "" // the body has no match; the metadata did
		}
		h.Snippet = strings.Join(strings.Fields(h.Snippet), " ")
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// ftsQuery turns user input into a safe FTS5 expression: every term is
// quoted (so punctuation and operators are literal) and terms are ANDed.
func ftsQuery(q string) string {
	var terms []string
	for len(q) > 0 {
		q = strings.TrimLeft(q, " \t\n")
		if q == "" {
			break
		}
		if q[0] == '"' {
			end := strings.IndexByte(q[1:], '"')
			phrase := q[1:]
			if end >= 0 {
				phrase, q = q[1:end+1], q[end+2:]
			} else {
				q = ""
			}
			if p := strings.TrimSpace(phrase); p != "" {
				terms = append(terms, quote(p))
			}
			continue
		}
		end := strings.IndexAny(q, " \t\n")
		word := q
		if end >= 0 {
			word, q = q[:end], q[end:]
		} else {
			q = ""
		}
		if strings.Trim(word, `"`) != "" {
			terms = append(terms, quote(strings.Trim(word, `"`))+"*")
		}
	}
	return strings.Join(terms, " ")
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
