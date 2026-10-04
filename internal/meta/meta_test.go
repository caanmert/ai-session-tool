package meta

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
)

func TestStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data", "ais.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := model.Session{Tool: model.ToolClaude, ID: "a"}
	b := model.Session{Tool: model.ToolCodex, ID: "a"} // same id, other tool
	steps := []error{
		st.SetTitle(ctx, a, "  Auth refresh fix  "),
		st.SetPinned(ctx, a, true),
		st.Tag(ctx, a, []string{"#Bug", "api", "bug"}, nil),
		st.Tag(ctx, b, []string{"infra"}, nil),
		st.SetArchived(ctx, b, true),
		st.Tag(ctx, a, nil, []string{"API"}),
	}
	for i, err := range steps {
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	st.Close()

	st, err = Open(path) // survives reopening
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	all, err := st.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ga, gb := all["claude/a"], all["codex/a"]
	if ga.Title != "Auth refresh fix" || !ga.Pinned || ga.Archived || strings.Join(ga.Tags, ",") != "bug" {
		t.Errorf("claude/a = %+v", ga)
	}
	if gb.Title != "" || gb.Pinned || !gb.Archived || strings.Join(gb.Tags, ",") != "infra" {
		t.Errorf("codex/a = %+v", gb)
	}
	counts, err := st.TagCounts(ctx)
	if err != nil || counts["bug"] != 1 || counts["infra"] != 1 || len(counts) != 2 {
		t.Errorf("TagCounts = %v, %v", counts, err)
	}

	if err := st.SetTitle(ctx, a, ""); err != nil { // back to the tool's title
		t.Fatal(err)
	}
	if all, _ := st.All(ctx); all["claude/a"].Title != "" {
		t.Error("empty title should clear the rename")
	}
}

func TestNormalizeTag(t *testing.T) {
	for in, want := range map[string]string{"bug": "bug", "#Bug": "bug", "\tapi/v2\n": "api/v2", "été": "été", "a:b-c_d.e": "a:b-c_d.e"} {
		if got, err := NormalizeTag(in); err != nil || got != want {
			t.Errorf("NormalizeTag(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "#", "two words", "semi;colon", strings.Repeat("x", 41)} {
		if _, err := NormalizeTag(bad); err == nil {
			t.Errorf("NormalizeTag(%q) accepted", bad)
		}
	}
}

func TestApplySortsPinnedFirst(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	sessions := []model.Session{
		{Tool: model.ToolClaude, ID: "new", Title: "Newest", UpdatedAt: now},
		{Tool: model.ToolCodex, ID: "old", Title: "Oldest", UpdatedAt: now.Add(-48 * time.Hour)},
		{Tool: model.ToolClaude, ID: "mid", Title: "Middle", UpdatedAt: now.Add(-time.Hour)},
	}
	Apply(sessions, map[string]Annotation{
		"codex/old":  {Pinned: true, Title: "Pinned one", Tags: []string{"keep"}},
		"claude/mid": {Archived: true},
	})
	var order []string
	for _, s := range sessions {
		order = append(order, s.ID)
	}
	if strings.Join(order, ",") != "old,new,mid" {
		t.Errorf("order = %v", order)
	}
	if s := sessions[0]; s.Title != "Pinned one" || s.OriginalTitle != "Oldest" || !s.Pinned || s.Tags[0] != "keep" {
		t.Errorf("annotated session = %+v", s)
	}
	if !sessions[2].Archived || sessions[2].OriginalTitle != "" {
		t.Errorf("archived session = %+v", sessions[2])
	}
}
