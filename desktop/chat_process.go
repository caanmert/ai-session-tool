package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type jsonObject map[string]json.RawMessage

func object(raw json.RawMessage) jsonObject {
	var value jsonObject
	_ = json.Unmarshal(raw, &value)
	return value
}
func str(raw json.RawMessage) string { var value string; _ = json.Unmarshal(raw, &value); return value }
func array(raw json.RawMessage) []json.RawMessage {
	var value []json.RawMessage
	_ = json.Unmarshal(raw, &value)
	return value
}
func number(raw json.RawMessage) int   { var value int; _ = json.Unmarshal(raw, &value); return value }
func boolean(raw json.RawMessage) bool { var value bool; _ = json.Unmarshal(raw, &value); return value }
func pretty(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

type wirePacket struct {
	Value jsonObject
	Err   error
}
type tailWriter struct {
	mu   sync.Mutex
	text string
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.text += string(p)
	if len(w.text) > 8000 {
		w.text = w.text[len(w.text)-8000:]
	}
	return len(p), nil
}
func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.text)
}

type jsonProcess struct {
	cmd     *exec.Cmd
	input   io.WriteCloser
	output  io.ReadCloser
	packets chan wirePacket
	stderr  tailWriter
	writeMu sync.Mutex
	once    sync.Once
	cancel  context.CancelFunc
	closed  chan struct{}
}

func startJSONProcess(ctx context.Context, cmd *exec.Cmd) (*jsonProcess, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	p := &jsonProcess{cmd: cmd, packets: make(chan wirePacket, 16), cancel: cancel, closed: make(chan struct{})}
	var err error
	p.input, err = cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	p.output, err = cmd.StdoutPipe()
	if err != nil {
		_ = p.input.Close()
		cancel()
		return nil, err
	}
	cmd.Stderr = &p.stderr
	if err = cmd.Start(); err != nil {
		_ = p.input.Close()
		_ = p.output.Close()
		cancel()
		return nil, err
	}
	go func() {
		defer close(p.packets)
		scanner := bufio.NewScanner(p.output)
		scanner.Buffer(make([]byte, 65536), 4<<20)
		for scanner.Scan() {
			var value jsonObject
			if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
				select {
				case p.packets <- wirePacket{Err: errors.New("agent returned invalid JSON; update the installed CLI")}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case p.packets <- wirePacket{Value: value}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case p.packets <- wirePacket{Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			p.close()
		case <-p.closed:
		}
	}()
	return p, nil
}
func (p *jsonProcess) send(value any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return json.NewEncoder(p.input).Encode(value)
}
func (p *jsonProcess) close() {
	p.once.Do(func() {
		p.cancel()
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		_ = p.input.Close()
		_ = p.output.Close()
		_ = p.cmd.Wait()
		close(p.closed)
	})
}
func (p *jsonProcess) failure() error {
	detail := p.stderr.String()
	if detail == "" {
		detail = "check that the installed CLI supports structured sessions and is signed in"
	}
	return fmt.Errorf("agent connection ended before the response completed: %s", detail)
}
