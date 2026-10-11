package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/creack/pty"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const terminalBufferSize = 1024 * 1024
const terminalLimit = 8

type LiveSession struct {
	ID       string `json:"id"`
	Tool     string `json:"tool"`
	Project  string `json:"project"`
	Key      string `json:"key"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
}

type TerminalChunk struct {
	Data     []byte `json:"data"`
	Cursor   int64  `json:"cursor"`
	Lost     bool   `json:"lost"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
}

type liveTerminal struct {
	mu            sync.Mutex
	inputMu       sync.Mutex
	info          LiveSession
	cmd           *exec.Cmd
	file          *os.File
	ring          [terminalBufferSize]byte
	end           int64
	processExited bool
	done          chan struct{}
	waited        chan struct{}
}

type terminals struct {
	mu     sync.Mutex
	items  map[string]*liveTerminal
	closed bool
}

func newTerminals() *terminals { return &terminals{items: make(map[string]*liveTerminal)} }

func (m *terminals) start(cmd *exec.Cmd, tool, key string) (LiveSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return LiveSession{}, errors.New("the app is closing")
	}
	for _, t := range m.items {
		t.mu.Lock()
		info := t.info
		t.mu.Unlock()
		if key != "" && info.Key == key && !info.Exited {
			return info, nil
		}
	}
	if len(m.items) >= terminalLimit {
		return LiveSession{}, errors.New("close an existing live tab before opening another (maximum 8)")
	}
	if err := launch.Check(cmd); err != nil {
		return LiveSession{}, err
	}
	// Execute the provider's argv directly. Never evaluate transcript text as shell code.
	env := cmd.Environ()
	cmd.Env = nil
	for _, item := range env {
		name, _, _ := strings.Cut(item, "=")
		switch name {
		case "TERM", "COLORTERM", "TERM_PROGRAM", "NO_COLOR", "CLICOLOR", "TMUX", "TMUX_PANE":
			continue
		}
		cmd.Env = append(cmd.Env, item)
	}
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=ais")
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 100})
	if err != nil {
		return LiveSession{}, fmt.Errorf("start terminal: %w", err)
	}
	// Use bounded nonblocking syscalls below so input, resize and close cannot hang
	// behind a full PTY. Access to the descriptor is protected against close/reuse.
	if err := unix.SetNonblock(int(file.Fd()), true); err != nil {
		_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		_ = file.Close()
		_ = cmd.Wait()
		return LiveSession{}, err
	}
	t := &liveTerminal{cmd: cmd, file: file, done: make(chan struct{}), waited: make(chan struct{}), info: LiveSession{ID: uuid.NewString(), Tool: tool, Project: cmd.Dir, Key: key}}
	m.items[t.info.ID] = t
	info := t.info
	go t.read()
	go func() {
		err := cmd.Wait()
		t.mu.Lock()
		t.processExited = true
		if err != nil {
			t.info.ExitCode = -1
			if cmd.ProcessState != nil {
				t.info.ExitCode = cmd.ProcessState.ExitCode()
			}
		}
		t.mu.Unlock()
		close(t.waited)
	}()
	return info, nil
}

func (t *liveTerminal) append(data []byte) {
	for len(data) > 0 {
		pos := int(t.end % terminalBufferSize)
		n := copy(t.ring[pos:], data)
		t.end += int64(n)
		data = data[n:]
	}
}

func (t *liveTerminal) read() {
	defer close(t.done)
	data := make([]byte, 32768)
	for {
		t.mu.Lock()
		if t.file == nil {
			t.mu.Unlock()
			break
		}
		n, err := unix.Read(int(t.file.Fd()), data)
		if n > 0 {
			t.append(data[:n])
		}
		// Drain bytes already produced before reporting exit to the UI.
		finished := (n <= 0 && err != unix.EAGAIN && err != unix.EINTR) || (n <= 0 && t.processExited)
		t.mu.Unlock()
		if finished {
			break
		}
		if n <= 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.mu.Lock()
	if t.file != nil {
		_ = t.file.Close()
		t.file = nil
	}
	t.mu.Unlock()
	// An agent can close its terminal before it exits. Keep ownership until Wait
	// completes; Stop and application shutdown can still terminate that process.
	<-t.waited
	t.mu.Lock()
	t.info.Exited = true
	t.mu.Unlock()
}

func (m *terminals) get(id string) (*liveTerminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.items[id]
	if t == nil {
		return nil, errors.New("live session not found")
	}
	return t, nil
}

func (m *terminals) list() []LiveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []LiveSession{}
	for _, t := range m.items {
		t.mu.Lock()
		result = append(result, t.info)
		t.mu.Unlock()
	}
	return result
}

func (m *terminals) poll(id string, cursor int64) (TerminalChunk, error) {
	t, err := m.get(id)
	if err != nil {
		return TerminalChunk{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if cursor < 0 || cursor > t.end {
		return TerminalChunk{}, errors.New("invalid terminal cursor")
	}
	start := max(int64(0), t.end-terminalBufferSize)
	lost := cursor < start
	if lost {
		cursor = start
	}
	count := min(t.end-cursor, 65536)
	data := make([]byte, int(count))
	pos := int(cursor % terminalBufferSize)
	n := copy(data, t.ring[pos:])
	copy(data[n:], t.ring[:])
	next := cursor + count
	return TerminalChunk{Data: data, Cursor: next, Lost: lost, Exited: t.info.Exited && next == t.end, ExitCode: t.info.ExitCode}, nil
}

func (m *terminals) input(id, data string) error {
	if len(data) > 65536 {
		return errors.New("terminal input is too large")
	}
	t, err := m.get(id)
	if err != nil {
		return err
	}
	t.inputMu.Lock()
	defer t.inputMu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for len(data) > 0 {
		t.mu.Lock()
		if t.file == nil || t.processExited {
			t.mu.Unlock()
			return errors.New("this terminal has exited")
		}
		n, err := unix.Write(int(t.file.Fd()), []byte(data))
		t.mu.Unlock()
		if n > 0 {
			data = data[n:]
		}
		if err != nil && err != unix.EAGAIN && err != unix.EINTR {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("terminal input timed out; the agent is not reading input")
		}
		if n <= 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return nil
}

func (m *terminals) resize(id string, cols, rows int) error {
	if cols < 2 || cols > 1000 || rows < 2 || rows > 1000 {
		return errors.New("invalid terminal size")
	}
	t, err := m.get(id)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil || t.processExited {
		return nil
	}
	return pty.Setsize(t.file, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (t *liveTerminal) stop() error {
	var stopErr error
	kill := func(pgid int) {
		if err := unix.Kill(-pgid, unix.SIGKILL); err != nil && err != unix.ESRCH {
			stopErr = errors.Join(stopErr, err)
		}
	}
	t.mu.Lock()
	if !t.processExited {
		// Agents may put a foreground job in another process group. Stop that group
		// as well as the session leader, then hang up the controlling terminal.
		if t.file != nil {
			if pgid, err := unix.IoctlGetInt(int(t.file.Fd()), unix.TIOCGPGRP); err == nil && pgid > 0 && pgid != unix.Getpgrp() {
				kill(pgid)
			}
		}
		kill(t.cmd.Process.Pid)
	}
	t.mu.Unlock()
	// Reader drains final output and closes the PTY after Wait. Bound cleanup even
	// if a misbehaving descendant retained a terminal descriptor.
	select {
	case <-t.done:
		return stopErr
	case <-time.After(3 * time.Second):
		t.mu.Lock()
		if t.file != nil {
			_ = t.file.Close()
			t.file = nil
		}
		t.mu.Unlock()
		return errors.Join(stopErr, errors.New("agent has not exited yet; try stopping it again"))
	}
}

func (m *terminals) stop(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	return t.stop()
}
func (m *terminals) remove(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	t.mu.Lock()
	exited := t.info.Exited
	t.mu.Unlock()
	if !exited {
		return errors.New("stop the live session before closing its tab")
	}
	m.mu.Lock()
	delete(m.items, id)
	m.mu.Unlock()
	return nil
}
func (m *terminals) close() {
	m.mu.Lock()
	m.closed = true
	items := make([]*liveTerminal, 0, len(m.items))
	for _, t := range m.items {
		items = append(items, t)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, t := range items {
		wg.Add(1)
		go func() { defer wg.Done(); _ = t.stop() }()
	}
	wg.Wait()
}
