package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/google/uuid"
)

func (a *App) chatReady() error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.chatError != nil {
		return a.chatError
	}
	if a.chats == nil {
		return errors.New("conversation manager is starting")
	}
	return nil
}
func (a *App) NewChat(project, tool string) (ChatState, error) {
	if err := a.chatReady(); err != nil {
		return ChatState{}, err
	}
	cmd, err := a.service.NewSession(tool, project)
	if err != nil {
		return ChatState{}, err
	}
	if err = launch.Check(cmd); err != nil {
		return ChatState{}, err
	}
	return a.chats.create(project, tool, "", "", nil)
}
func (a *App) ResumeChat(key string) (ChatState, error) {
	if err := a.chatReady(); err != nil {
		return ChatState{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	cmd, err := a.service.Resume(ctx, key)
	if err != nil {
		return ChatState{}, err
	}
	if err = launch.Check(cmd); err != nil {
		return ChatState{}, err
	}
	tool, id, ok := strings.Cut(key, "/")
	if !ok {
		return ChatState{}, errors.New("invalid conversation key")
	}
	preview, err := a.service.Transcript(ctx, key)
	if err != nil {
		return ChatState{}, err
	}
	messages := []ChatMessage{}
	for _, message := range preview.Messages {
		kind := "tool"
		if string(message.Kind) == "text" {
			kind = "text"
		}
		messages = append(messages, ChatMessage{ID: uuid.NewString(), Role: string(message.Role), Kind: kind, Text: message.Text, Name: message.ToolName})
	}
	state, err := a.chats.create(cmd.Dir, tool, id, key, messages)
	if err != nil {
		return state, err
	}
	c, _ := a.chats.get(state.ID)
	c.mu.Lock()
	c.state.Shortened = c.state.Shortened || preview.Truncated
	if c.state.Title == "New conversation" {
		c.state.Title = "Continued " + tool + " conversation"
	}
	c.change()
	state = cloneState(c.state)
	c.mu.Unlock()
	a.chats.saveOrWarn(c)
	return state, nil
}
func (a *App) ListChats() ([]ChatState, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	return a.chats.list(), nil
}
func (a *App) ReadChat(id string, version int64) (ChatUpdate, error) {
	if err := a.chatReady(); err != nil {
		return ChatUpdate{}, err
	}
	return a.chats.read(id, version)
}
func (a *App) SendChat(id, text string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.send(id, text)
}
func (a *App) StopChat(id string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.stop(id)
}
func (a *App) CloseChat(id string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.remove(id)
}
func (a *App) AnswerChat(id, request string, allow bool, answers map[string]string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.approve(id, request, allow, answers)
}
func (a *App) PrepareReview(id string) (ReviewDraft, error) {
	if err := a.chatReady(); err != nil {
		return ReviewDraft{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.prepareReview(ctx, id)
}
func (a *App) StartReview(id string) (ChatState, error) {
	if err := a.chatReady(); err != nil {
		return ChatState{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.startReview(ctx, id)
}
func (a *App) SendReviewFindings(id string, findings []string) (string, error) {
	if err := a.chatReady(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.sendFindings(ctx, id, findings)
}

func (a *App) ChatModels(id string, reviewer, refresh bool) (ModelCatalog, error) {
	if err := a.chatReady(); err != nil {
		return ModelCatalog{}, err
	}
	c, err := a.chats.get(id)
	if err != nil {
		return ModelCatalog{}, err
	}
	c.mu.Lock()
	tool, project := c.state.Tool, c.state.Project
	c.mu.Unlock()
	if reviewer {
		tool = otherProvider(tool)
		project = ""
	}
	return a.chats.models(a.ctx, tool, project, refresh)
}
func (a *App) SetChatSettings(id string, settings ModelSettings, reviewer bool) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.settings(a.ctx, id, settings, reviewer)
}
