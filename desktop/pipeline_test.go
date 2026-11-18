package main

import (
	"strings"
	"testing"
)

func TestPipelinePlannedByClaude(t *testing.T) {
	m := fakeChats(t, "pipeline")
	project := t.TempDir()
	for _, bad := range []struct{ task, planner string }{{" ", "claude"}, {"Add retries", "other"}} {
		if _, err := m.startPipeline(project, bad.task, bad.planner); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	plan, err := m.startPipeline(project, "Add retries\nto checkout", "claude")
	if err != nil {
		t.Fatal(err)
	}
	link := plan.Pipeline
	if plan.Tool != "claude" || plan.Mode != modePlan || plan.Title != "Plan: Add retries to checkout" || link == nil || link.Step != "plan" || link.Builder != "codex" {
		t.Fatalf("plan conversation: %+v %+v", plan, link)
	}
	done := waitChat(t, m, plan.ID, func(s ChatState) bool { return s.Status == "idle" })
	if done.Mode != modePlan || len(done.Approvals) != 0 {
		t.Fatalf("the planner stays in plan mode and nothing waits: %+v", done)
	}
	view, err := m.pipeline(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Step != "planned" || view.Plan != "1. Add retries\n2. Test them" || view.PlanChat != plan.ID || view.Builder != "codex" {
		t.Fatalf("planned: %+v", view)
	}
	if _, err := m.buildPipeline(link.ID, " "); err == nil {
		t.Fatal("an empty plan must not be built")
	}
	build, err := m.buildPipeline(link.ID, view.Plan+"\n3. EDITED by the user")
	if err != nil {
		t.Fatal(err)
	}
	if build.Tool != "codex" || build.Title != "Build: Add retries to checkout" || build.Pipeline.Step != "build" || build.Pipeline.PlanChat != plan.ID || build.Mode != modeDefault {
		t.Fatalf("build conversation: %+v %+v", build, build.Pipeline)
	}
	if _, err := m.buildPipeline(link.ID, "again"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("a second build: %v", err)
	}
	waitChat(t, m, build.ID, func(s ChatState) bool { return s.Status == "idle" && len(s.Messages) >= 2 })
	view, _ = m.pipeline(link.ID)
	if view.Step != "built" || view.BuildChat != build.ID || !strings.Contains(view.Plan, "EDITED") {
		t.Fatalf("built: %+v", view)
	}
	// The planner reviews the build with the usual review.
	review := createChat(t, m, project, "claude", "")
	r, _ := m.get(review.ID)
	r.mu.Lock()
	r.state.Review = &ReviewState{SourceID: build.ID, Complete: true, Findings: []ReviewFinding{{Title: "a"}, {Title: "b"}}}
	r.mu.Unlock()
	view, _ = m.pipeline(link.ID)
	if view.Step != "reviewed" || view.ReviewChat != review.ID || view.Findings != 2 {
		t.Fatalf("reviewed: %+v", view)
	}
	r.mu.Lock()
	r.state.Status = "running"
	r.mu.Unlock()
	if view, _ = m.pipeline(link.ID); view.Step != "reviewing" {
		t.Fatalf("reviewing: %+v", view)
	}
}

func TestPipelinePlannedByCodex(t *testing.T) {
	m := fakeChats(t, "pipeline")
	plan, err := m.startPipeline(t.TempDir(), "Add retries", "codex")
	if err != nil {
		t.Fatal(err)
	}
	id := plan.Pipeline.ID
	waitChat(t, m, plan.ID, func(s ChatState) bool { return s.Status == "idle" && len(s.Messages) >= 2 })
	view, _ := m.pipeline(id)
	if view.Step != "planned" || view.Plan != "Hello from codex" || view.Builder != "claude" {
		t.Fatalf("Codex's plan is its reply: %+v", view)
	}
	c, _ := m.get(plan.ID)
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if view, _ = m.pipeline(id); view.Step != "planning" {
		t.Fatalf("planning: %+v", view)
	}
	if _, err := m.buildPipeline(id, "EDITED"); err == nil || !strings.Contains(err.Error(), "wait for the plan") {
		t.Fatalf("building while planning: %v", err)
	}
	// Planning that ended without a reply has no plan to build.
	c.mu.Lock()
	c.state.Status = "stopped"
	c.state.Messages = c.state.Messages[:1]
	c.mu.Unlock()
	if view, _ = m.pipeline(id); view.Step != "unplanned" || view.Plan != "" {
		t.Fatalf("unplanned: %+v", view)
	}
	if _, err := m.pipeline("pl-missing"); err == nil {
		t.Fatal("an unknown task")
	}
}

func TestLastReply(t *testing.T) {
	messages := []ChatMessage{
		{Role: "user", Kind: "text", Text: "Plan"},
		{Role: "assistant", Kind: "text", Text: " The plan "},
		{Role: "assistant", Kind: "tool", Text: "ran"},
	}
	if got := lastReply(messages); got != "The plan" {
		t.Fatalf("got %q", got)
	}
	if got := lastReply(append(messages, ChatMessage{Role: "user", Kind: "text", Text: "More"})); got != "" {
		t.Fatalf("after a newer message: %q", got)
	}
}
