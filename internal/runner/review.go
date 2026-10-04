package runner

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
)

// FileChange describes a net change from the run's starting commit. Untracked
// files use "?". Renames are shown as a deletion and an addition.
type FileChange struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type TaskStatus struct {
	Task          Task         `json:"task"`
	State         string       `json:"state"` // running, exited, missing, unknown (tmux pane only)
	Pane          string       `json:"pane,omitempty"`
	ExitCode      *int         `json:"exitCode,omitempty"`
	Signal        string       `json:"signal,omitempty"`
	Attention     bool         `json:"attention"`
	Quiet         bool         `json:"quiet"`
	CurrentBranch string       `json:"currentBranch,omitempty"`
	Files         []FileChange `json:"files"`
	Error         string       `json:"error,omitempty"`
}

type RunStatus struct {
	ID      string       `json:"id"`
	Base    string       `json:"base"`
	Tasks   []TaskStatus `json:"tasks"`
	Warning string       `json:"warning,omitempty"`
}

// SelectTasks validates an optional task name before any inspection or attach.
func SelectTasks(r Run, name string) (Run, error) {
	if name == "" {
		return r, nil
	}
	for _, task := range r.Tasks {
		if task.Window == name {
			r.Tasks = []Task{task}
			return r, nil
		}
	}
	return Run{}, fmt.Errorf("run %s has no task %q", r.ID, name)
}

const paneFormat = "#{pane_id}\t#{pane_dead}\t#{pane_dead_status}\t#{pane_dead_signal}\t#{window_bell_flag}\t#{window_silence_flag}\t#{@ais_task}\t#{window_name}"

func (m *Manager) panes(ctx context.Context, r Run) (map[string]TaskStatus, error) {
	if _, err := m.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux unavailable: %w", err)
	}
	out, err := m.Command(ctx, "", "tmux", "list-panes", "-s", "-t", "="+r.ID, "-F", paneFormat)
	if err != nil {
		// Only explicit absence is a missing session. Permissions, malformed
		// output and other inspection failures must remain unknown.
		msg := err.Error()
		if ctx.Err() == nil && (strings.Contains(msg, "can't find session:") ||
			strings.Contains(msg, "can't find window:") ||
			strings.Contains(msg, "no server running on ") ||
			(strings.Contains(msg, "error connecting to ") && strings.Contains(msg, "No such file or directory"))) {
			return map[string]TaskStatus{}, nil
		}
		return nil, err
	}
	states := map[string]TaskStatus{}
	if out == "" {
		return states, nil
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 8)
		if len(fields) != 8 || !strings.HasPrefix(fields[0], "%") || (fields[1] != "0" && fields[1] != "1") {
			return nil, errors.New("invalid tmux pane status")
		}
		name := fields[6]
		if name == "" {
			name = fields[7]
		} // manifests launched before pane tags
		st := TaskStatus{Pane: fields[0], State: "running", Attention: fields[4] == "1", Quiet: fields[5] == "1"}
		if fields[1] == "1" {
			st.State, st.Signal = "exited", fields[3]
			if fields[2] != "" {
				code, err := strconv.Atoi(fields[2])
				if err != nil {
					return nil, fmt.Errorf("invalid tmux exit status: %w", err)
				}
				st.ExitCode = &code
			}
		}
		// A tagged agent pane takes priority over extra panes in the same
		// window. For legacy runs with multiple panes, keep the first pane.
		if _, exists := states[name]; !exists || fields[6] != "" {
			states[name] = st
		}
	}
	return states, nil
}

// Status inspects tmux once and each worktree independently. A missing worktree
// or unavailable tmux must not hide the remaining tasks or imply success.
func (m *Manager) Status(ctx context.Context, r Run) (RunStatus, error) {
	res := RunStatus{ID: r.ID, Base: r.Base, Tasks: []TaskStatus{}}
	panes, paneErr := m.panes(ctx, r)
	if paneErr != nil {
		res.Warning = paneErr.Error()
	}
	for _, task := range r.Tasks {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		st, ok := panes[task.Window]
		if !ok {
			st.State = "missing"
		}
		if paneErr != nil {
			st.State = "unknown"
		}
		st.Task = task
		st.Files = []FileChange{}
		if err := checkWorktree(ctx, task.Path); err != nil {
			st.Error = err.Error()
		} else {
			branch, err := gitOutput(ctx, task.Path, "rev-parse", "--abbrev-ref", "HEAD")
			if err == nil {
				st.CurrentBranch = strings.TrimSuffix(string(branch), "\n")
				if st.CurrentBranch == "HEAD" {
					st.CurrentBranch = "(detached)"
				}
				st.Files, err = changedFiles(ctx, task.Path, r.Base)
			}
			if err != nil {
				st.Error = err.Error()
			}
		}
		res.Tasks = append(res.Tasks, st)
	}
	return res, ctx.Err()
}

// TaskAttachCmd targets the tagged agent pane, even after its window is renamed.
func (m *Manager) TaskAttachCmd(ctx context.Context, r Run, name string, inside bool) (*exec.Cmd, error) {
	selected, err := SelectTasks(r, name)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return AttachCmd(r, inside), nil
	}
	panes, err := m.panes(ctx, selected)
	if err != nil {
		return nil, err
	}
	st, ok := panes[name]
	if !ok {
		return nil, fmt.Errorf("task %s has no tmux pane; its worktree is %s", name, selected.Tasks[0].Path)
	}
	cmd := AttachCmd(r, inside)
	cmd.Args[len(cmd.Args)-1] = st.Pane
	return cmd, nil
}

// gitOutput preserves NUL framing and patch whitespace, and keeps diagnostics
// out of successful stdout. Optional index refresh writes are disabled.
func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--no-optional-locks"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return out, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(exit.Stderr)))
		}
		return out, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

func checkWorktree(ctx context.Context, path string) error {
	root, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	want, err = filepath.Abs(want)
	if err != nil {
		return err
	}
	got, err := filepath.EvalSymlinks(strings.TrimSuffix(string(root), "\n"))
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s is no longer the task's worktree root", path)
	}
	return nil
}

func changedFiles(ctx context.Context, path, base string) ([]FileChange, error) {
	out, err := gitOutput(ctx, path, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-status", "-z", base, "--")
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(out, []byte{0})
	if len(fields)%2 != 1 || len(fields[len(fields)-1]) != 0 {
		return nil, errors.New("invalid Git changed-file output")
	}
	files := []FileChange{}
	for i := 0; i+1 < len(fields); i += 2 {
		files = append(files, FileChange{Status: string(fields[i]), Path: string(fields[i+1])})
	}
	untracked, err := untrackedFiles(ctx, path)
	if err != nil {
		return nil, err
	}
	for _, name := range untracked {
		files = append(files, FileChange{Status: "?", Path: name})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func untrackedFiles(ctx context.Context, path string) ([]string, error) {
	out, err := gitOutput(ctx, path, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if out[len(out)-1] != 0 {
		return nil, errors.New("invalid Git untracked-file output")
	}
	return strings.Split(string(out[:len(out)-1]), "\x00"), nil
}

// Diff includes committed and uncommitted changes against the saved base, plus
// untracked files. It neither stages files nor invokes external diff drivers.
func (m *Manager) Diff(ctx context.Context, r Run, task Task, stat bool) (string, error) {
	return m.diff(ctx, r, task, stat, "")
}

// DiffFile uses a literal repository-relative path, including for untracked
// files. An empty path selects the whole task.
func (m *Manager) DiffFile(ctx context.Context, r Run, task Task, file string) (string, error) {
	if file != "" && (filepath.IsAbs(file) || filepath.Clean(file) == ".." || strings.HasPrefix(filepath.Clean(file), ".."+string(filepath.Separator))) {
		return "", errors.New("diff path must stay within the task worktree")
	}
	return m.diff(ctx, r, task, false, file)
}

func (m *Manager) diff(ctx context.Context, r Run, task Task, stat bool, file string) (string, error) {
	if err := checkWorktree(ctx, task.Path); err != nil {
		return "", err
	}
	options := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--src-prefix=a/", "--dst-prefix=b/"}
	if stat {
		options = append(options, "--stat")
	}
	args := append(append([]string{}, options...), r.Base, "--")
	if file != "" {
		args = append(args, ":(literal)"+file)
	}
	out, err := gitOutput(ctx, task.Path, args...)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	result.Write(out)
	untracked, err := untrackedFiles(ctx, task.Path)
	if err != nil {
		return "", err
	}
	for _, name := range untracked {
		if file != "" && name != file {
			continue
		}
		args := append(append([]string{}, options...), "--no-index", "--", os.DevNull, name)
		out, err := gitOutput(ctx, task.Path, args...)
		var exit *exec.ExitError
		if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1 || len(exit.Stderr) != 0) {
			return "", err
		}
		result.Write(out)
	}
	return result.String(), ctx.Err()
}
