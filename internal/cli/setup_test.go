package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
)

func setupApp(t *testing.T, present ...string) (*App, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	var out bytes.Buffer
	app := &App{
		Out: &out, Err: &out, In: strings.NewReader(""),
		Providers: func() []provider.Provider {
			return []provider.Provider{claude.New(filepath.Join(root, "claude")), codex.New(filepath.Join(root, "codex"))}
		},
		LookPath: func(name string) (string, error) {
			for _, found := range present {
				if found == name {
					return "/tools/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		IndexPath: func() (string, error) { t.Fatal("setup opened the index"); return "", nil },
		DataDir:   func() (string, error) { t.Fatal("setup opened the data store"); return "", nil },
	}
	return app, &out, root
}

func TestSetupFreshInstallDoesNotWriteOrRequireTools(t *testing.T) {
	app, out, root := setupApp(t)
	cmd := NewRootCmd(app)
	cmd.SetArgs([]string{"setup"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[missing] tmux: parallel tasks only", "Not created yet", "waiting for your first saved conversation", "optional for browsing", "CODEX_HOME", "CLAUDE_CONFIG_DIR"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("setup wrote to session storage: %v, %v", entries, err)
	}
}

func TestSetupSavedSessionsWithoutAgent(t *testing.T) {
	app, out, root := setupApp(t, "git", "tmux")
	project := filepath.Join(root, "claude", "projects", "example")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	// Setup only discovers files; parsing and live checks belong to doctor.
	if err := os.WriteFile(filepath.Join(project, "example.jsonl"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runSetup(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 transcript files available to index", "Resume: needs", "install claude or codex"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	app.LookPath = func(name string) (string, error) { return "/tools/" + name, nil }
	out.Reset()
	if err := runSetup(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Resume: CLI and transcript files found for claude", "Parallel tasks: tools found", "setup does not verify"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}

func TestSetupInvalidRootAndCancellation(t *testing.T) {
	app, out, root := setupApp(t)
	if err := os.WriteFile(filepath.Join(root, "codex"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runSetup(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Expected a directory. Check CODEX_HOME") || !strings.Contains(out.String(), "fix the session location problems") {
		t.Fatal(out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runSetup(ctx, app); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled setup: %v", err)
	}
}

func TestDependencyHintsMatchPlatform(t *testing.T) {
	for _, tc := range []struct {
		tool, platform string
		brew           bool
		want           string
	}{
		{"tmux", "darwin", true, "brew install tmux"},
		{"codex", "darwin", true, "brew install --cask codex"},
		{"git", "darwin", false, "xcode-select --install"},
		{"lsof", "darwin", false, "/usr/sbin"},
		{"tmux", "linux", false, "system package manager"},
		{"claude", "linux", true, "https://code.claude.com"},
	} {
		if got := dependencyHint(tc.tool, tc.platform, tc.brew); !strings.Contains(got, tc.want) {
			t.Errorf("%+v: %s", tc, got)
		}
	}
}

func TestDoctorIncludesOptionalDependencies(t *testing.T) {
	app, out, _ := setupApp(t)
	app.Providers = func() []provider.Provider { return nil }
	app.IndexPath, app.DataDir = nil, nil
	if err := runDoctor(context.Background(), app, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[dependencies]", "[missing] git", "[missing] tmux", "[missing] lsof", "Fix:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}
