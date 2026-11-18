package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/google/uuid"
)

// A guided task runs plan → build → review across the two agents. One agent
// plans in Plan mode and changes nothing; the user reads and may edit the
// plan; the other agent builds it in a new, linked conversation; then the
// planner reviews the uncommitted changes with the usual review. Every step
// starts on the user's click, approvals work as in any conversation, and
// committing stays the user's. ais runs the steps itself; no model directs
// another.

// PipelineLink ties a conversation to its guided task.
type PipelineLink struct {
	ID string `json:"id"`
	// Step is plan or build: the part this conversation plays.
	Step    string `json:"step"`
	Task    string `json:"task"`
	Planner string `json:"planner"`
	Builder string `json:"builder"`
	// Plan, on the plan conversation, is the plan Claude Code proposed when
	// it asked to leave plan mode, and later the plan the build was given.
	Plan string `json:"plan,omitempty"`
	// Build, on the plan conversation, is the build conversation.
	Build string `json:"build,omitempty"`
	// PlanChat, on the build conversation, is the plan conversation.
	PlanChat string `json:"planChat,omitempty"`
}

// Pipeline is a guided task as it stands, worked out from its conversations.
type Pipeline struct {
	ID         string `json:"id"`
	Task       string `json:"task"`
	Project    string `json:"project"`
	Planner    string `json:"planner"`
	Builder    string `json:"builder"`
	PlanChat   string `json:"planChat"`
	BuildChat  string `json:"buildChat,omitempty"`
	ReviewChat string `json:"reviewChat,omitempty"`
	Plan       string `json:"plan,omitempty"`
	// Step is planning, unplanned (planning ended without a plan), planned,
	// building, built, reviewing or reviewed.
	Step string `json:"step"`
	// Findings counts the review's findings once it is complete.
	Findings int `json:"findings,omitempty"`
}

const (
	pipelineTaskLimit = 100000
	pipelinePlanLimit = 100000
)

// planPrompt is the planner's first message.
func planPrompt(task, builder string) string {
	return fmt.Sprintf("Plan this task. %s will build it from your plan after I review the plan, so don't change any files.\n\nTask:\n%s\n\nRead what you need in the project, then reply with a numbered, step-by-step plan: the files to change, what to change in each, and how to check that it works.", toolName(builder), task)
}

// buildPrompt is the builder's first message.
func buildPrompt(task, plan, planner string) string {
	return fmt.Sprintf("Implement this plan. %s wrote it and I approved it.\n\nTask:\n%s\n\nPlan:\n%s\n\nFollow the plan. If part of it turns out to be wrong, say so and do the closest sound thing. Finish by running the project's checks.", toolName(planner), task, plan)
}

// lastReply is the agent's last text after the user's last message.
func lastReply(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "user" {
			return ""
		}
		if m.Role == "assistant" && m.Kind == "text" && strings.TrimSpace(m.Text) != "" {
			return strings.TrimSpace(m.Text)
		}
	}
	return ""
}

// keepPlan records the plan Claude Code proposes when it asks to leave plan
// mode in a guided task's plan conversation. It reports whether it did, in
// which case the request is refused: the planner must not start building.
func (c *chatSession) keepPlan(plan string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	link := c.state.Pipeline
	if link == nil || link.Step != "plan" || link.Build != "" {
		return false
	}
	link.Plan = shorten(strings.TrimSpace(plan), pipelinePlanLimit)
	c.change()
	return true
}

// startPipeline opens the plan conversation and asks the planner for a plan.
func (m *chatManager) startPipeline(project, task, planner string) (ChatState, error) {
	task = strings.TrimSpace(task)
	switch {
	case task == "":
		return ChatState{}, errors.New("describe the task to plan")
	case len(task) > pipelineTaskLimit:
		return ChatState{}, errors.New("the task is longer than 100,000 bytes")
	case planner != "claude" && planner != "codex":
		return ChatState{}, errors.New("choose which agent plans")
	}
	builder := otherTool(planner)
	state, err := m.create(project, planner, "", "", nil)
	if err != nil {
		return ChatState{}, err
	}
	c, err := m.get(state.ID)
	if err != nil {
		return ChatState{}, err
	}
	c.mu.Lock()
	c.state.Mode = modePlan
	c.state.Title = "Plan: " + shorten(strings.Join(strings.Fields(task), " "), 74)
	c.state.Pipeline = &PipelineLink{ID: "pl-" + uuid.NewString(), Step: "plan", Task: task, Planner: planner, Builder: builder}
	c.change()
	c.mu.Unlock()
	if err := m.save(c); err != nil {
		_ = m.remove(state.ID)
		return ChatState{}, err
	}
	if err := m.send(state.ID, planPrompt(task, builder)); err != nil {
		_ = m.remove(state.ID)
		return ChatState{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneState(c.state), nil
}

// pipelineChats finds a guided task's plan and build conversations, and the
// latest review of the build.
func (m *chatManager) pipelineChats(id string) (plan, build, review *chatSession) {
	m.mu.Lock()
	sessions := make([]*chatSession, 0, len(m.items))
	for _, c := range m.items {
		sessions = append(sessions, c)
	}
	m.mu.Unlock()
	buildID := ""
	for _, c := range sessions {
		c.mu.Lock()
		if link := c.state.Pipeline; link != nil && link.ID == id {
			if link.Step == "plan" {
				plan = c
			} else if link.Step == "build" {
				build = c
				buildID = c.state.ID
			}
		}
		c.mu.Unlock()
	}
	if buildID == "" {
		return plan, build, nil
	}
	reviews := []*chatSession{}
	for _, c := range sessions {
		c.mu.Lock()
		if c.state.Review != nil && c.state.Review.SourceID == buildID {
			reviews = append(reviews, c)
		}
		c.mu.Unlock()
	}
	sort.Slice(reviews, func(i, j int) bool {
		reviews[i].mu.Lock()
		a := reviews[i].state.UpdatedAt
		reviews[i].mu.Unlock()
		reviews[j].mu.Lock()
		b := reviews[j].state.UpdatedAt
		reviews[j].mu.Unlock()
		return a.Before(b)
	})
	if len(reviews) > 0 {
		review = reviews[len(reviews)-1]
	}
	return plan, build, review
}

// pipeline works out where a guided task stands.
func (m *chatManager) pipeline(id string) (Pipeline, error) {
	plan, build, review := m.pipelineChats(id)
	if plan == nil {
		return Pipeline{}, errors.New("that task's plan conversation is gone")
	}
	plan.mu.Lock()
	link := *plan.state.Pipeline
	view := Pipeline{ID: id, Task: link.Task, Project: plan.state.Project, Planner: link.Planner, Builder: link.Builder, PlanChat: plan.state.ID}
	view.Plan = link.Plan
	if view.Plan == "" {
		view.Plan = lastReply(plan.state.Messages)
	}
	planBusy := busyChat(plan.state.Status)
	plan.mu.Unlock()
	switch {
	case build == nil && planBusy:
		view.Step = "planning"
	case build == nil && view.Plan == "":
		view.Step = "unplanned"
	case build == nil:
		view.Step = "planned"
	}
	if build != nil {
		build.mu.Lock()
		view.BuildChat = build.state.ID
		buildBusy := busyChat(build.state.Status)
		build.mu.Unlock()
		view.Step = "built"
		if buildBusy {
			view.Step = "building"
		}
	}
	if review != nil && view.Step == "built" {
		review.mu.Lock()
		view.ReviewChat = review.state.ID
		switch {
		case busyChat(review.state.Status):
			view.Step = "reviewing"
		case review.state.Review.Complete:
			view.Step = "reviewed"
			view.Findings = len(review.state.Review.Findings)
		}
		review.mu.Unlock()
	}
	return view, nil
}

// buildPipeline hands the (possibly edited) plan to the builder in a new,
// linked conversation and starts it.
func (m *chatManager) buildPipeline(id, plan string) (ChatState, error) {
	plan = strings.TrimSpace(plan)
	switch {
	case plan == "":
		return ChatState{}, errors.New("there is no plan to build yet")
	case len(plan) > pipelinePlanLimit:
		return ChatState{}, errors.New("the plan is longer than 100,000 bytes")
	}
	planChat, build, _ := m.pipelineChats(id)
	if planChat == nil {
		return ChatState{}, errors.New("that task's plan conversation is gone")
	}
	if build != nil {
		return ChatState{}, errors.New("this plan is already being built")
	}
	planChat.mu.Lock()
	link := *planChat.state.Pipeline
	project, title, status := planChat.state.Project, strings.TrimPrefix(planChat.state.Title, "Plan: "), planChat.state.Status
	planChat.mu.Unlock()
	if busyChat(status) {
		return ChatState{}, errors.New("wait for the plan to finish, or stop it, before building")
	}
	state, err := m.create(project, link.Builder, "", "", nil)
	if err != nil {
		return ChatState{}, err
	}
	c, err := m.get(state.ID)
	if err != nil {
		return ChatState{}, err
	}
	c.mu.Lock()
	c.state.Title = "Build: " + title
	c.state.Pipeline = &PipelineLink{ID: id, Step: "build", Task: link.Task, Planner: link.Planner, Builder: link.Builder, PlanChat: planChat.state.ID}
	c.change()
	c.mu.Unlock()
	planChat.mu.Lock()
	planChat.state.Pipeline.Build = state.ID
	planChat.state.Pipeline.Plan = plan
	planChat.change()
	planChat.mu.Unlock()
	undo := func() {
		planChat.mu.Lock()
		planChat.state.Pipeline.Build = ""
		planChat.state.Pipeline.Plan = link.Plan
		planChat.change()
		planChat.mu.Unlock()
		m.saveOrWarn(planChat)
		_ = m.remove(state.ID)
	}
	if err := m.save(c); err != nil {
		undo()
		return ChatState{}, err
	}
	m.saveOrWarn(planChat)
	if err := m.send(state.ID, buildPrompt(link.Task, plan, link.Planner)); err != nil {
		undo()
		return ChatState{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneState(c.state), nil
}

// checkAgent checks that an agent can start in a project.
func (a *App) checkAgent(tool, project string) error {
	cmd, err := a.service.NewSession(tool, project)
	if err != nil {
		return err
	}
	return launch.Check(cmd)
}

// StartPipeline starts a guided task: the planner plans it in Plan mode. Both
// agents must be installed, since the other one builds.
func (a *App) StartPipeline(project, task, planner string) (ChatState, error) {
	if err := a.chatReady(); err != nil {
		return ChatState{}, err
	}
	for _, tool := range []string{"claude", "codex"} {
		if err := a.checkAgent(tool, project); err != nil {
			return ChatState{}, err
		}
	}
	return a.chats.startPipeline(project, task, planner)
}

// PipelineView says where a guided task stands. Only reads.
func (a *App) PipelineView(id string) (Pipeline, error) {
	if err := a.chatReady(); err != nil {
		return Pipeline{}, err
	}
	return a.chats.pipeline(id)
}

// BuildPipeline gives the plan to the other agent in a new conversation and
// starts it.
func (a *App) BuildPipeline(id, plan string) (ChatState, error) {
	if err := a.chatReady(); err != nil {
		return ChatState{}, err
	}
	view, err := a.chats.pipeline(id)
	if err != nil {
		return ChatState{}, err
	}
	if err := a.checkAgent(view.Builder, view.Project); err != nil {
		return ChatState{}, err
	}
	return a.chats.buildPipeline(id, plan)
}
