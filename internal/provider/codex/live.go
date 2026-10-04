package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// Live identifies sessions by the rollout files held open by local Codex
// processes, including the shared daemon. A held-open session is conservatively
// considered live even when idle. It does not infer activity from file age.
func (p *Provider) Live(ctx context.Context) ([]model.LiveState, error) {
	root, err := filepath.EvalSymlinks(p.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	read := p.OpenFiles
	if read == nil {
		read = codexOpenFiles
	}
	data, err := read(ctx)
	if err != nil {
		return nil, fmt.Errorf("codex live state unavailable: %w", err)
	}
	states := map[string]model.LiveState{}
	pid := 0
	// -F0 terminates fields with NUL; lsof also inserts newlines between
	// process/file sets. NUL framing preserves spaces and newlines in paths.
	for _, field := range bytes.Split(data, []byte{0}) {
		field = bytes.TrimLeft(field, "\n")
		if len(field) < 2 {
			continue
		}
		switch field[0] {
		case 'p':
			pid, _ = strconv.Atoi(string(field[1:]))
		case 'n':
			if pid <= 0 {
				continue
			}
			path := strings.TrimSuffix(string(field[1:]), " (deleted)")
			rel, err := filepath.Rel(root, path)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(rel, "sessions"+string(filepath.Separator)) &&
				!strings.HasPrefix(rel, "archived_sessions"+string(filepath.Separator)) {
				continue
			}
			name, ok := parseRolloutName(filepath.Base(path))
			if !ok {
				continue
			}
			if _, exists := states[name.thread]; !exists {
				states[name.thread] = model.LiveState{PID: pid, SessionID: name.thread, Status: "running"}
			}
		}
	}
	out := make([]model.LiveState, 0, len(states))
	for _, st := range states {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return out, nil
}

func codexOpenFiles(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsof", "-n", "-P", "-F0pn", "-a", "-u", strconv.Itoa(os.Getuid()), "-c", "codex")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var exit *exec.ExitError
	// lsof returns 1 without diagnostics when no matching process exists.
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(data) == 0 && stderr.Len() == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lsof: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stderr.Len() > 0 {
		return nil, fmt.Errorf("lsof returned an incomplete view: %s", strings.TrimSpace(stderr.String()))
	}
	return data, nil
}
