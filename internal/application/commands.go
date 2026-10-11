package application

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
)

func DefaultProviders() []provider.Provider {
	return []provider.Provider{claude.New(claude.DefaultRoot()), codex.New(codex.DefaultRoot())}
}

// Commands are constructed as argument arrays, never by evaluating transcript text.
// Callers may print them without requiring installed agents; launch.Check is
// required before execution.
func NewCommand(providers []provider.Provider, tool, directory string) (*exec.Cmd, error) {
	if err := ValidateTool(tool); err != nil {
		return nil, err
	}
	p := provider.For(providers, model.Tool(tool))
	if p == nil {
		return nil, fmt.Errorf("choose an installed agent: claude or codex")
	}
	dir, err := Directory(directory)
	if err != nil {
		return nil, err
	}
	return p.NewCmd(dir), nil
}

func ResumeCommand(providers []provider.Provider, session model.Session) (*exec.Cmd, error) {
	if session.Archived {
		return nil, errors.New("this session is archived; unarchive it before continuing")
	}
	if session.CWD == "" {
		return nil, errors.New("this session has no recorded project directory")
	}
	if _, err := Directory(session.CWD); err != nil {
		return nil, err
	}
	p := provider.For(providers, session.Tool)
	if p == nil {
		return nil, fmt.Errorf("no provider for %s", session.Tool)
	}
	return p.ResumeCmd(session, false)
}
