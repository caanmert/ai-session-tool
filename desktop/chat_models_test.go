package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModelCatalogsAndValidation(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := fakeChats(t, "normal")
			project := t.TempDir()
			catalog, err := m.models(context.Background(), provider, project, false)
			if err != nil {
				t.Fatal(err)
			}
			if catalog.DefaultModel != "model-a" {
				t.Fatal("missing default model")
			}
			if err = catalog.validate(ModelSettings{Model: "model-b", Effort: "high"}); err != nil {
				t.Fatal(err)
			}
			if catalog.validate(ModelSettings{Model: "missing"}) == nil {
				t.Fatal("accepted unavailable model")
			}
			if catalog.validate(ModelSettings{Model: "model-b", Effort: "ultra"}) == nil {
				t.Fatal("accepted unsupported effort")
			}
			if provider == "codex" && len(catalog.Models) != 2 {
				t.Fatal("missing paginated model")
			}
			if provider == "claude" && catalog.validate(ModelSettings{Model: "fast-model", Effort: "high"}) == nil {
				t.Fatal("invented effort support")
			}
			if catalog.validate(ModelSettings{}) != nil {
				t.Fatal("default should be usable")
			}
			// A cached lookup must not launch another process.
			m.command = nil
			if _, err = m.models(context.Background(), provider, project, false); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestModelSettingsPersistenceAndResume(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := fakeChats(t, "settings")
			source := createChat(t, m, t.TempDir(), provider, "native-thread")
			selected := ModelSettings{Model: "model-b", Effort: "high"}
			if err := m.settings(context.Background(), source.ID, selected, false); err != nil {
				t.Fatal(err)
			}
			if err := m.settings(context.Background(), source.ID, ModelSettings{Effort: "low"}, true); err != nil {
				t.Fatal(err)
			}
			reloaded := newChats(m.dataDir)
			if err := reloaded.load(); err != nil {
				t.Fatal(err)
			}
			saved, _ := reloaded.read(source.ID, -1)
			if saved.State.Settings.Model != "model-b" || saved.State.Settings.Effort != "high" || saved.State.ReviewerSettings.Effort != "low" {
				t.Fatal("preferences were not kept separately")
			}
			if err := m.send(source.ID, "continue"); err != nil {
				t.Fatal(err)
			}
			if err := m.settings(context.Background(), source.ID, ModelSettings{}, false); err == nil {
				t.Fatal("settings changed during response")
			}
			done := waitChat(t, m, source.ID, func(s ChatState) bool { return !busyChat(s.Status) })
			if done.Status != "idle" || done.ActiveModel != "model-b" || done.Messages[1].Model != "model-b" {
				t.Fatalf("selected model not used/reported: %+v", done)
			}
		})
	}
}
func TestDefaultSettingsAndReportedFallback(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		for _, mode := range []string{"default-settings", "reroute"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				m := fakeChats(t, mode)
				source := createChat(t, m, t.TempDir(), provider, "native-thread")
				if mode == "default-settings" {
					if err := m.settings(context.Background(), source.ID, ModelSettings{}, false); err != nil {
						t.Fatal(err)
					}
				}
				if err := m.send(source.ID, "continue"); err != nil {
					t.Fatal(err)
				}
				done := waitChat(t, m, source.ID, func(s ChatState) bool { return !busyChat(s.Status) })
				want := "model-a"
				if mode == "reroute" {
					want = "model-fallback"
				}
				if done.Status != "idle" || done.Messages[1].Model != want {
					t.Fatalf("incorrect response model: %+v", done)
				}
			})
		}
	}
}
func TestReviewerUsesSeparateSettings(t *testing.T) {
	m := fakeChats(t, "review")
	source := createChat(t, m, reviewRepo(t), "codex", "")
	if err := m.settings(context.Background(), source.ID, ModelSettings{Model: "model-b", Effort: "high"}, false); err != nil {
		t.Fatal(err)
	}
	if err := m.settings(context.Background(), source.ID, ModelSettings{Model: "fast-model"}, true); err != nil {
		t.Fatal(err)
	}
	draft, err := m.prepareReview(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Settings.Model != "fast-model" {
		t.Fatal("preview lost reviewer settings")
	}
	review, err := m.startReview(context.Background(), draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	done := waitChat(t, m, review.ID, func(s ChatState) bool { return !busyChat(s.Status) })
	if done.Status != "idle" || done.Settings.Model != "fast-model" || done.Settings.Effort != "" {
		t.Fatalf("review inherited coding settings: %+v", done)
	}
}
func TestMissingModelCatalog(t *testing.T) {
	m := fakeChats(t, "no-models")
	source := createChat(t, m, t.TempDir(), "claude", "")
	if _, err := m.models(context.Background(), "claude", source.Project, false); err == nil {
		t.Fatal("accepted empty model catalog")
	}
	if err := m.settings(context.Background(), source.ID, ModelSettings{}, false); err != nil {
		t.Fatal("cannot clear settings with unavailable catalog")
	}
	if err := m.settings(context.Background(), source.ID, ModelSettings{Model: "unknown"}, false); err == nil {
		t.Fatal("unavailable model saved")
	}
}

// Opt-in compatibility check: initialization/catalog queries only, never a model turn.
func TestInstalledModelCatalogs(t *testing.T) {
	if os.Getenv("AIS_TEST_INSTALLED_MODELS") != "1" {
		t.Skip("set AIS_TEST_INSTALLED_MODELS=1 for installed CLI discovery")
	}
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			defer m.close()
			catalog, err := m.models(context.Background(), provider, "", true)
			if err != nil {
				t.Fatal(err)
			}
			if len(catalog.Models) == 0 {
				t.Fatal("empty catalog")
			}
			t.Logf("%s: %d models, default %s", provider, len(catalog.Models), catalog.DefaultModel)
		})
	}
}

func TestModelSettingsRollbackOnSaveFailure(t *testing.T) {
	m := fakeChats(t, "normal")
	source := createChat(t, m, t.TempDir(), "claude", "")
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	m.dataDir = blocked
	if err := m.settings(context.Background(), source.ID, ModelSettings{Model: "model-b", Effort: "high"}, false); err == nil {
		t.Fatal("save should fail")
	}
	update, _ := m.read(source.ID, -1)
	if update.State.Settings.Configured || update.State.Settings.Model != "" {
		t.Fatal("failed save changed active settings")
	}
}
func TestCanceledModelDiscovery(t *testing.T) {
	m := fakeChats(t, "normal")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := m.models(ctx, "codex", t.TempDir(), false); err == nil {
		t.Fatal("canceled discovery succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation was delayed")
	}
	m.close()
	if _, err := m.models(context.Background(), "claude", "", false); err == nil {
		t.Fatal("discovery started after shutdown")
	}
}
