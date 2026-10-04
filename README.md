# ais: AI session manager

Find, preview and resume your **Claude Code** and **Codex CLI** sessions from anywhere.

Both tools save every conversation on disk, but finding and resuming one is clumsy:
- `claude --resume` only lists sessions for the directory you're in.
- Codex buries its sessions in date folders.

`ais` reads both stores and gives you one list across all your projects. From that list you can resume any session in its own project directory.

Read the [usage guide](docs/USAGE.md) for setup, keyboard shortcuts, a complete parallel-task workflow, and troubleshooting.

> **Status:** browse, search, resume, fork, rename, tag, pin, archive and trash Claude Code and Codex CLI sessions, see what they used, and launch parallel tasks in separate worktrees and tmux windows with agent event alerts.

## Install

```sh
go install github.com/caanmert/ai-session-tool/cmd/ais@latest
```

Requires Go 1.24+. Prebuilt macOS binaries are published on each tagged release.

## Interactive browser

Run `ais` with no arguments in a terminal:

- the list shows every session, newest first; the pane below previews the selected one (your prompts, replies rendered as Markdown, tool calls)
- `/` filters as you type: words match the title, project, branch, id, model, your tags and anything said in the conversation; `t:codex`, `p:api`, `b:main`, `#tag`, `is:live`, `is:pinned` and `is:archived` narrow it down
- `enter` resumes the session in its own project directory, `f` forks it, `n` starts a new one in the same project. When the agent exits you are back in the list
- `r` renames, `t` tags (`bug +auth -old`), `p` pins to the top, `a` archives (hidden unless `is:archived`), `d` moves to the trash after asking
- `s` swaps the preview for usage stats of whatever the filter shows (by day, then model, then project)
- `y` copies the resume command, `tab` scrolls the preview, `ctrl+r` rescans, `?` lists every key, `q` quits

Piped (`ais | head`), it prints the recent list instead.

## Usage

```sh
ais                          # interactive browser (or the recent list when piped)
ais ls                       # newest first: id, tool, project, branch, age, messages, tokens, title
ais ls -p api --since 7d     # filter by project path and recency
ais ls --live                # only sessions whose agent is running right now
ais ls --json | jq .         # scriptable output

ais search webhook retry     # full-text: titles, prompts, replies and tool calls, best match first
ais search '"exact phrase"' --tool codex --json

ais show 3f2a                # details + transcript (any unique id prefix works)
ais show 3f2a --tools        # include tool output
ais show 3f2a --info         # details only

ais resume 3f2a              # cd into the session's project and run `claude --resume <id>` / `codex resume <id>`
ais resume 3f2a --fork       # continue in a new session, keep the original (`--fork-session` / `codex fork`)
ais resume 3f2a --print      # print the command instead: cd '/path' && claude --resume …

ais doctor                   # where ais looks, how many sessions it found, parse warnings
ais reindex                  # rebuild the index from scratch (always safe)

ais rename 3f2a Token refresh fix     # your own title (--reset restores the tool's)
ais tag 3f2a bug auth                 # ais tags lists them; ls --tag bug filters
ais pin 3f2a / ais unpin 3f2a         # pinned sessions stay on top
ais archive 3f2a / ais unarchive 3f2a # hidden from lists; ls --all shows them
ais trash 3f2a                        # remove from the tool too; ais trash lists the trash
ais restore 3f2a                      # bring it back
ais trash --empty --older-than 30d    # delete for good (asks first)

ais stats                             # tokens and ≈ cost per day, last 30 days
ais stats --by model --since all      # also: week, month, project, tool; --json
```

On a terminal, output is colored. Each tool has its own color, recent sessions stand out, and `show` draws a session card with replies rendered as Markdown (code blocks, lists, emphasis). Piped or redirected output stays plain text, so `grep`, `awk` and `--json` scripts keep working. Use `--color=always|never|auto` to override; `NO_COLOR` is honored. Light and dark terminal backgrounds are detected; set `AIS_THEME=dark` or `light` if your terminal doesn't report its background.

Example:

```
   ID        TOOL    PROJECT  BRANCH         UPDATED  MSGS  TOKENS  TITLE
   aaaaaaaa  codex   api      feat/webhooks  21h      4     21.2k   Webhook retries
●  22222222  claude  web      main           2d       3     2k      Dark mode
   11111111  claude  api      feat/auth      3d       5     4.2k    Fix auth token refresh
```

`●` marks a session whose agent is running, including idle Codex sessions that still have their rollout open.

## How it works

Browsing and indexing only **read** the tools' files and local process metadata. Annotations stay in ais's own database; explicit trash/restore operations move files or ask the tool to archive/unarchive them, as described below.

| Tool | Transcripts | Running sessions | Override |
| --- | --- | --- | --- |
| Claude Code | `~/.claude/projects/<project>/<session-id>.jsonl` | `~/.claude/sessions/<pid>.json` | `CLAUDE_CONFIG_DIR` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl[.zst]` | open rollout files (`lsof`) | `CODEX_HOME` |

For each transcript, `ais` collects:
- the working directory, git branch, model and version;
- prompt and reply counts;
- token usage, counting each API response once;
- a title: the session's own name (Claude's custom title, Codex's `session_index.jsonl`), else Claude's summary, else your first prompt.

Codex specifics:
- Rollouts older than a week are zstd-compressed (`.jsonl.zst`); `ais` reads both, preferring the plain file when both exist.
- A reverted thread gets a new rollout file that points at the earlier one through `history_base`; `ais` lists the newest file and stitches the history back together.
- Subagent and internal threads are hidden; archived threads (`archived_sessions/`) are not listed.
- Mixed transcript formats are combined message by message, including across `history_base` rollouts.
- Live detection requires `lsof` on macOS or Linux and includes Codex daemon sessions. Inspection failures appear in `ais doctor`; trash and permanent deletion are blocked if live state cannot be checked.

Lines it can't parse are skipped and reported by `ais doctor`.

### Index

Parsed sessions are cached in a SQLite index (pure Go, no cgo) at `~/Library/Caches/ais/index.db` on macOS (`~/.cache/ais` on Linux; override with `AIS_CACHE_DIR`). Each run re-reads only transcripts whose size or modification time changed, so the list appears instantly even with thousands of sessions. On 2,000 sessions (600 MB of transcripts) the first build took 9 s and later runs about 0.1 s. A direct scan takes 3.6 s every time. The index also holds the conversation text for `ais search` and the browser's filter.

It is only a cache: delete it or run `ais reindex` at any time. `--no-index` reads transcripts directly.

### Your data and the trash

Titles, tags, pins and archive flags you set are yours: they live in `~/Library/Application Support/ais/ais.db` (`~/.local/share/ais` on Linux; override with `AIS_DATA_DIR`), keyed by tool and session id, and are never written into Claude's or Codex's files.

The trash removes a session from the tool itself, restorably:
- **Claude Code** sessions (the transcript and its folder of subagent data) are moved into `ais`'s trash folder, and moved back on `ais restore`.
- **Codex** keeps its own database, so `ais` calls `codex archive` / `codex unarchive`, and `codex delete` when you empty the trash.

Live state is checked again immediately before trashing or permanently deleting a session; sessions detected as running are refused. Codex restore records are saved before archiving. If archiving is interrupted, the pending record remains in `ais trash`, can be recovered with `ais restore <id>`, and cannot be permanently deleted until resolved.

### Usage and cost

`ais stats` adds up the tokens each API response reports, split by 15-minute slot and model, so a session that spans days or mixes models (Claude subagents often run on Haiku) is counted where and when it happened. Codex records running totals, so each step's increase is attributed to the model in use at the time.

Cost is an estimate at API list prices, shown as `≈`; on a subscription you pay a flat fee instead. Current Claude models are priced out of the box, including the higher price of one-hour cache writes that Claude Code uses. Codex's OpenAI models have no built-in price. Add any model in `~/.config/ais/config.toml` (or `$AIS_CONFIG`):

```toml
[prices."gpt-5.5-codex"]   # a model id or prefix; the longest match wins
input = 1.25               # USD per million tokens
output = 10.0
cache_read = 0.125
# cache_write and cache_write_1h default to 1.25x and 2x input
```

Rows that include unpriced models show a `+`; `—` means nothing in the row had a price.

Resuming runs the agent **in the session's original directory**, because `claude --resume` only finds sessions of the current project. On macOS and Linux, `ais` replaces itself with the agent process, so the agent gets the terminal exactly as if you had started it yourself.

## Parallel tasks

Requires `git`, `tmux`, and the chosen agent on `PATH`. Give each task as a
separate quoted argument (use `--` before the prompts):

```sh
ais run --tool claude -- "Fix login validation" "Add export tests"
ais run --tool codex --project /path/to/repo -- "Improve search" "Document the API"
ais run --notify=false -- "A task without ais event alerts"
ais run --quiet 30s -- "A task with optional silence alerts too"
ais run list                 # saved runs, including stopped runs
ais run list --json          # prompts, branches and worktree paths
ais run status ais-1234      # pane states, alerts, branches and changed files
ais run status ais-1234 --json
ais run review ais-1234      # interactive tasks, changed files and diffs
ais run diff ais-1234        # review every task's changes from the starting commit
ais run diff ais-1234 --task task-2 --stat
ais run attach ais-1234      # full run id or unique prefix
ais run attach ais-1234 --task task-2 # jump directly to a task
ais run stop ais-1234        # terminate the run's agents; preserve their work
```

Each run creates a detached tmux session with one window per task. Every task
gets its own new branch and worktree at the same committed `HEAD` of the chosen
project. Uncommitted changes in your original checkout are not copied. Agents
use their normal permission settings. There is no automatic merge or commit.

Agents receive the caller's `PATH`, `CODEX_HOME`, `CODEX_SQLITE_HOME` and
`CLAUDE_CONFIG_DIR`, including when a variable is unset, even if the tmux server
was started with different settings.

`ais run attach` joins the session, or switches to it if you are already inside
tmux. With tmux's default keys, `ctrl+b n` moves to the next task and `ctrl+b d`
detaches while agents keep running. Exited panes retain their output.

### Review a run

`ais run review <id>` combines status, changed files and diffs in one interactive
screen. It refreshes every three seconds, preserving the selected task/file and
diff scroll position. Use `--task task-2` to select an initial task, or
`--refresh 0s` for manual refresh. Wide terminals show files beside the diff;
narrow terminals stack the panels.

- `tab` / `shift+tab` move between tasks, files and diff panels.
- Arrow keys or `j/k` select a task/file or scroll the diff. `pgup/pgdn` and
  `g/G` move faster; left/right or `h/l` pan long diff lines.
- Select **All changes** for the whole task, or a file for its individual patch.
- `a` attaches to the selected agent from any panel. `enter` also attaches,
  except in the files panel, where it focuses the diff.
- `r` or `ctrl+r` refreshes; `?` opens help; `q` quits review and leaves agents running.

Outside tmux, detach with `ctrl+b d` to return to review. Inside tmux, switch back
to the review window/session. Large patch previews are truncated; `ais run diff`
provides the complete output. Interactive review requires a terminal; use the
following commands for scripts and redirected output.

`ais run status <id>` shows each task's pane state (`running`, `exited`,
`missing`, or `unknown`), exit code when available, outstanding event/silence
alerts, current branch and changed files. `--task task-2` selects one task;
`--json` includes file paths and any inspection errors. Missing worktrees are
reported individually, and Git inspection still works if tmux is unavailable.
Pane state describes the local terminal process; a daemon may continue work
after its client exits. An outstanding alert is an event, not proof that the
agent is still waiting.

`ais run diff <id>` compares each worktree with the run's saved starting commit,
including committed, staged and unstaged changes, plus untracked files. Ignored
files are excluded. `--stat` shows file statistics; `--task task-2` limits the
review to one task. Renames appear as deletions and additions. Both review
commands leave files and the Git index unchanged and work after a run stops.
If a task cannot be read, `diff` reports the error, continues reviewing the other
tasks, and exits unsuccessfully. Running tasks can change files during review;
stop or pause their work before treating a diff as final.

Use `ais run attach <id> --task task-2` to jump to its agent pane. Newly launched
runs retain this mapping when you rename a tmux window; older runs use the
original window name.

### Agent notifications

New runs enable event alerts by default (`--notify=false` opts out):

- **Codex:** completed turns and approval requests use its built-in terminal
  notifications, configured to emit a bell even when the terminal has focus.
- **Claude Code:** a per-invocation `Notification` hook forwards idle prompts,
  permission prompts and elicitation dialogs to the originating tmux pane.
  Claude normally waits about 60 seconds before an idle alert, or six seconds
  for permission/elicitation alerts; typing and background work can delay them.

tmux marks the window with its bell indicator and shows a message when you are
viewing another window in that run. Selecting the window clears the indicator.
An alert records an event that needs attention; it is not a continuously updated
busy/waiting status. These alerts work inside tmux and do not send desktop
notifications while you are detached.

Configuration is passed to each agent on launch. No user or project settings
files are changed, and existing hooks and approval policies remain in effect.
Claude hooks must be enabled and allowed by your settings. Older agents that
do not support these settings can use `--notify=false`. The integration follows
[Codex terminal notifications](https://learn.chatgpt.com/docs/config-file/config-advanced#notifications)
and [Claude Notification hooks](https://code.claude.com/docs/en/hooks#notification).

Silence alerts are disabled by default. Add `--quiet 30s` to highlight a window
after 30 seconds without output. Silence means **possibly waiting for input**;
an agent can also be silently working. Use `--quiet 0s` to disable that timer.
Existing running sessions retain the settings they were launched with.

Run manifests and worktrees live in `<ais data directory>/runs/<run-id>/`.
Manifests are saved before agents start, including all planned worktree paths,
so a setup failure can be inspected with `ais run list --json`. Stopping a run
closes its tmux session; it keeps branches, worktrees and manifests. Review and
merge or cherry-pick the changes yourself. When finished, use `git worktree
remove <path>` and `git branch -d <branch>` to clean up safely. Do not delete
ais's data directory as if it were the rebuildable index cache: it holds your
task work as well as annotations.

## Roadmap

1. [x] Scaffold: Go module, Cobra CLI, lint, CI, goreleaser
2. [x] Claude Code adapter: `ls`, `show`, `resume`, `doctor`
3. [x] Codex adapter
4. [x] SQLite index (incremental) + full-text `ais search`
5. [x] Bubble Tea TUI: list + preview, filter, resume / fork / new
6. [x] Tags, rename, pin, archive/trash + restore
7. [x] Token/cost stats by day, week, month, project, model, tool
8. [x] Parallel runner: tmux windows + git worktrees, saved runs, attach/stop and quiet-window alerts
9. [x] Agent event alerts in tmux: Codex completion/approval and Claude idle/permission/elicitation notifications
10. [x] Run status, task-specific attach, and diffs for reviewing parallel work
11. [x] Interactive run review with task status, file selection, diff previews and agent attach

## Development

```sh
go test ./...                         # unit + golden tests on fixtures in testdata/
go test ./internal/cli -update        # accept changed golden output
golangci-lint run
go run ./cmd/ais doctor               # try it against your real sessions
```

Test fixtures under `testdata/` are made-up transcripts in each tool's real on-disk format. The Codex format was taken from the open-source [openai/codex](https://github.com/openai/codex) (`codex-rs/rollout`, `codex-rs/protocol`).
