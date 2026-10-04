# Using ais

`ais` helps you find existing Claude Code and Codex sessions, run independent tasks in separate Git worktrees, and review what each agent changed.

This guide describes the current checkout, including the interactive run review screen.

## 1. Get started

From this project's directory, use the local binary:

```sh
./ais --help
./ais doctor
./ais
```

To rebuild it, install Go 1.24.7 or newer and run:

```sh
go build -o ais ./cmd/ais
```

To install the current checkout as a command:

```sh
go install ./cmd/ais
```

Make sure Go's executable installation directory is on your `PATH`. The examples below use `ais`; substitute `./ais` when using the local binary from the project directory. After changing directories, use the installed command or the binary's absolute path.

Different operations need different tools:

| Operation | Requirements |
| --- | --- |
| Browse saved sessions | Local Claude Code or Codex transcript files |
| Resume a session | The corresponding agent CLI, configured for normal use |
| Launch parallel tasks | `git`, `tmux`, and the chosen agent CLI on `PATH`; a Git repository with at least one commit |
| Detect live Codex sessions | `lsof` on `PATH` |
| Interactive browser or run review | A terminal for both input and output |
| Review a stopped run's changes | Git and the saved worktrees; the agents do not need to be running |

Set up and sign in to the agent CLI you plan to use before launching tasks. `ais` uses that tool's normal configuration and permission settings.

## 2. Find and resume an existing session

Open the session browser:

```sh
ais
```

Use the arrow keys to choose a session. Its conversation appears in the preview. Press **Enter** to resume it in its recorded project directory, or **f** to fork it into a new conversation.

| Key | Session browser action |
| --- | --- |
| `↑` / `↓` or `j` / `k` | Select a session |
| `/` | Filter sessions |
| `Enter` | Resume the selected session |
| `f` | Fork the selected session |
| `n` | Start a new session in the same project |
| `Tab` | Switch between the list and preview |
| `s` | Show usage statistics; press again to change grouping |
| `Ctrl+r` | Rescan sessions |
| `?` | Show keyboard help |
| `q` | Quit the browser |

Try these expressions after pressing `/`:

| Filter | Finds |
| --- | --- |
| `webhook retry` | Sessions matching those words |
| `t:codex` | Codex sessions |
| `p:my-project` | Sessions in matching project paths |
| `b:main` | Sessions on a matching branch |
| `#bug` | Sessions tagged `bug` |
| `is:live` | Sessions detected as live |
| `is:pinned` | Pinned sessions |
| `is:archived` | Sessions hidden using ais's archive flag |

You can combine filters, for example `t:codex p:my-project webhook`.

For terminal commands or scripts:

```sh
ais ls --tool codex
ais ls --live
ais search webhook retry
ais show 3f2a
ais resume 3f2a
ais resume 3f2a --fork
ais resume 3f2a --print
```

Here, `3f2a` is an example session ID prefix. Replace it with an ID from `ais ls`; the prefix must identify one session. `--print` shows the resume command without starting the agent.

## 3. Launch independent tasks

Choose tasks that can be developed separately, such as documentation and tests. Each task starts from the same committed `HEAD`. **Uncommitted changes in your original checkout are not copied.** Commit any changes that the agents need before launching a run.

For Codex:

```sh
ais run --tool codex --project "/path/to/repository" -- \
  "Improve the README installation instructions" \
  "Add regression tests for input validation"
```

For Claude Code:

```sh
ais run --tool claude --project "/path/to/repository" -- \
  "Improve the README installation instructions" \
  "Add regression tests for input validation"
```

Replace `/path/to/repository` with your project's path. Keep each prompt in quotes and put `--` before the prompts. Omitting `--project` uses the current directory; omitting `--tool` selects Claude.

`ais` creates a branch and worktree for each prompt, then starts the agents in separate windows of a detached tmux session. It prints the run ID, branches, and paths.

There are three names to keep track of:

| Name | Example | Used for |
| --- | --- | --- |
| Session ID | `3f2a…` | `ais show`, `resume`, session annotations and trash |
| Run ID | `ais-a1b2c3d4e5f6` | `ais run status`, `review`, `attach`, `diff`, `stop` |
| Task name | `task-1` | Selecting one task within a run |

The following examples use **`ais-a1b2c3d4e5f6` as a placeholder**. Replace it with the ID printed for your run. Run commands also accept a unique run ID prefix. Git branch names require their actual full names.

Find saved runs at any time:

```sh
ais run list
ais run list --json
```

The JSON output includes the task prompts, branch names and worktree paths. Saved runs remain listed after their tmux session stops.

## 4. Monitor and review the work

Open the interactive review screen:

```sh
ais run review ais-a1b2c3d4e5f6
```

The top panel lists tasks with their pane state, alerts, changed-file count and branch. The files panel selects either **All changes** or one file. The diff panel shows the corresponding patch, with additions and deletions colored when color is enabled.

| Key | Run review action |
| --- | --- |
| `Tab` / `Shift+Tab` | Move between tasks, files and diff panels |
| `↑` / `↓` or `j` / `k` | Select a task/file, or scroll the diff |
| `PgUp` / `PgDn` | Move through the current panel faster |
| `g` / `G` | Go to the first/last item or top/bottom of the diff |
| `←` / `→` or `h` / `l` | Scroll the diff horizontally |
| `Enter` in the files panel | Focus the selected file's diff |
| `Enter` in another panel, or `a` anywhere | Attach to the selected task |
| `r` / `Ctrl+r` | Refresh status and the selected diff |
| `?` | Show help |
| `Esc` | Close help and focus the task list |
| `q` | Quit review; agents keep running |

Review refreshes every three seconds and preserves the selected task/file and scroll position. Wide terminals show files beside the diff; narrower terminals stack them.

```sh
ais run review ais-a1b2c3d4e5f6 --task task-2
ais run review ais-a1b2c3d4e5f6 --refresh 5s
ais run review ais-a1b2c3d4e5f6 --refresh 0s
```

In `review`, `--task` selects the initial task; you can still browse the others. `--refresh 0s` switches to manual refresh. Very large previews are truncated; use `ais run diff` for complete output.

For a non-interactive view:

```sh
ais run status ais-a1b2c3d4e5f6
ais run status ais-a1b2c3d4e5f6 --json
ais run diff ais-a1b2c3d4e5f6 --stat
ais run diff ais-a1b2c3d4e5f6 --task task-2
```

In `status` and `diff`, `--task` limits the output to that task.

Changes are compared with the run's saved starting commit, including committed, staged and unstaged changes, plus untracked files. Ignored files are excluded. Renames appear as a deletion and an addition. Review does not stage, commit or merge anything. These commands also work after a run stops, while its worktrees remain available.

### Read the state and alerts

| Display | Meaning |
| --- | --- |
| `running` | The task's tmux pane process is running |
| `exited (0)` | The pane process exited successfully |
| `exited` with another code or signal | Inspect its retained terminal output for the reason |
| `missing` | The task's pane or run session was not found |
| `unknown` | Pane inspection failed or tmux is unavailable |
| `event` | An agent event triggered a tmux bell alert |
| `silence` | The optional no-output timer triggered |
| Files unavailable | Git could not inspect that worktree; an error is shown |

Pane state describes the terminal process. It does not prove that a daemon-backed agent has finished its work. Alerts record events; they are not a continuously updated waiting/busy status.

## 5. Answer an agent or provide another instruction

Press `a` in review, or attach directly:

```sh
ais run attach ais-a1b2c3d4e5f6
ais run attach ais-a1b2c3d4e5f6 --task task-2
```

You are now interacting with the agent's normal terminal UI. Answer its questions, respond to approval prompts, or provide another instruction there.

With tmux's default bindings, press `Ctrl+b`, release it, then press:

| Key after `Ctrl+b` | Action |
| --- | --- |
| `n` | Next task window |
| `p` | Previous window |
| `w` | Choose a window |
| `d` | Detach, leaving the agents running |

If you entered an agent from review **outside tmux**, detaching returns you to review. If review was already **inside tmux**, attach switches the tmux client; switch back to the review window/session to return.

### Notification settings

New runs enable agent event alerts by default. Codex reports turn completion and approval requests. Claude forwards its idle, permission and elicitation notifications; its hooks must be enabled, and the agent may delay those notifications.

tmux marks the affected window and can show a message while you view another window in that run. Selecting the window clears its alert indicator. These are tmux alerts, not desktop notifications while detached.

```sh
ais run --tool codex --notify=false -- "Improve the installation guide"
ais run --tool codex --quiet 30s -- "Improve the installation guide"
```

`--notify=false` disables ais's event-alert setup. `--quiet 30s` adds a silence alert after 30 seconds without output. Silence alerts default to off because an agent may be working without producing output. Settings apply when the run starts; they do not reconfigure existing runs.

## 6. Keep the changes you want

Wait for the agents to finish or stop their work through the agent UI, then inspect the final diffs and run the project's relevant tests in each task worktree. `ais run list --json` gives you the worktree paths and branch names.

If accepted changes are still uncommitted, commit them in their task worktree. Replace the paths below with the actual worktree and reviewed files:

```sh
git -C "/path/to/task-worktree" status --short
git -C "/path/to/task-worktree" diff
git -C "/path/to/task-worktree" add -- path/to/reviewed-file
git -C "/path/to/task-worktree" diff --cached
git -C "/path/to/task-worktree" commit -m "Improve installation instructions"
```

If the agent already committed the accepted work, inspect those commits instead of creating a duplicate commit.

From a clean checkout of your original repository, you can create an integration branch and merge the task branches:

```sh
git switch -c review-agent-work
git merge ais-a1b2c3d4e5f6/task-1
git merge ais-a1b2c3d4e5f6/task-2
```

Use the full branch names from your actual run. Resolve any conflicts and test the combined result before merging it into your usual development branch or opening a pull request. You can also cherry-pick selected commits instead. `ais` does not perform these integration steps automatically.

## 7. Stop and clean up a run

Close the run's tmux session when you no longer need its terminal windows:

```sh
ais run stop ais-a1b2c3d4e5f6
```

This preserves branches, worktrees and the run manifest. A daemon-backed agent may continue work after its terminal client exits, so finish or stop its work in the agent before relying on the files being final.

After the changes are committed and integrated, remove the worktrees and merged branches using their actual paths and names:

```sh
git worktree remove "/path/to/task-1-worktree"
git branch -d ais-a1b2c3d4e5f6/task-1
```

Repeat for the other tasks. If Git refuses removal because work is uncommitted or a branch is unmerged, inspect and preserve that work first. Once a worktree is removed, ais cannot show its working-tree diff. The saved run manifest remains available in `ais run list`.

## 8. Organize sessions and inspect usage

Session annotations use session IDs, not run IDs:

```sh
ais rename 3f2a "Installation guide"
ais tag 3f2a docs review
ais pin 3f2a
ais ls --tag docs
ais tags
ais stats
ais stats --by model --since all
```

Token costs are estimates. Models without configured prices are not assigned an invented cost; see [Usage and cost in the README](../README.md#usage-and-cost) for price configuration.

There are two different ways to put a session away:

| Command | Effect | Undo |
| --- | --- | --- |
| `ais archive 3f2a` | Sets ais's hidden/archive annotation | `ais unarchive 3f2a` |
| `ais trash 3f2a` | Moves Claude files to trash, or asks Codex to archive its thread | `ais restore 3f2a` |

Use `ais ls --all` to include sessions hidden by ais's archive annotation. Use `ais trash` to list trash entries. Active sessions are refused by the trash operation; live state is checked again before mutation.

```sh
ais trash
ais restore 3f2a
ais trash --empty --older-than 30d
```

The last command asks before permanently deleting eligible entries. A Codex entry marked as an incomplete archive keeps its restore record; follow the recovery instruction shown with the error rather than deleting that record.

## 9. Storage and configuration

| Data | Default location | Override |
| --- | --- | --- |
| Claude transcripts | `~/.claude/projects/` | `CLAUDE_CONFIG_DIR` changes the Claude root |
| Codex transcripts | `~/.codex/sessions/` | `CODEX_HOME` changes the Codex root |
| ais data on macOS | `~/Library/Application Support/ais/` | `AIS_DATA_DIR` |
| ais data on Linux | `$XDG_DATA_HOME/ais/`, otherwise `~/.local/share/ais/` | `AIS_DATA_DIR` |
| Session index on macOS | `~/Library/Caches/ais/index.db` | `AIS_CACHE_DIR` changes the cache directory |
| Session index on Linux | `$XDG_CACHE_HOME/ais/index.db`, otherwise `~/.cache/ais/index.db` | `AIS_CACHE_DIR` changes the cache directory |

Run manifests and task worktrees are under `<ais data directory>/runs/<run-id>/`. The same data directory also holds annotations and trash. **It contains your work, so it is not disposable cache.** The session index is rebuildable with `ais reindex`.

Changing `AIS_DATA_DIR` changes which saved runs ais sees. Runner launches forward the caller's `PATH`, `CODEX_HOME`, `CODEX_SQLITE_HOME` and `CLAUDE_CONFIG_DIR`, including unset values, to avoid stale tmux-server settings.

## 10. Troubleshooting

| Problem | What to check |
| --- | --- |
| No sessions appear | Run `ais doctor`; confirm the Claude/Codex roots and transcript files |
| Session results look outdated | Run `ais reindex`, or compare with `ais --no-index ls` |
| `ais` is not found | Use `./ais` from this checkout, or put the installed binary on `PATH` |
| `tmux` or an agent is missing | Make that executable available on the launching shell's `PATH` |
| A task does not see local edits | Runs start from committed `HEAD`; uncommitted edits are not copied |
| Setup stopped partway through | Inspect `ais run list --json`; work is preserved and some agents may already be running |
| Review says it needs a terminal | Run it directly in a terminal; use `run status --json` or `run diff` in scripts |
| Pane state is unknown | Read the inspection warning; ensure tmux is available and accessible |
| A worktree is unavailable | Check its saved path; it may have been moved or removed |
| Codex live detection or trash is blocked | Ensure `lsof` can inspect local Codex processes; check `ais doctor` |
| No attention alert appears | Confirm the run started with notifications enabled; Claude hooks may be disabled or delayed |
| The diff preview is truncated | Use `ais run diff <run-id> --task task-1` for complete output |
| Terminal colors look wrong | Try `AIS_THEME=light ais`, `AIS_THEME=dark ais`, or `ais --color=never` |

For the exact options supported by your binary:

```sh
ais --help
ais run --help
ais run review --help
ais run status --help
ais run diff --help
```
