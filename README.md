# ais: AI session manager

Find, preview and resume your **Claude Code** and **Codex CLI** sessions from anywhere.

Both tools save every conversation on disk, but finding and resuming one is clumsy:
- `claude --resume` only lists sessions for the directory you're in.
- Codex buries its sessions in date folders.

`ais` reads both stores and gives you one list across all your projects. From that list you can resume any session in its own project directory.

> **Status:** early. Claude Code and Codex CLI sessions both work in the interactive browser and with `ls` / `show` / `resume` / `doctor`. A search index is next (see [Roadmap](#roadmap)).

## Install

```sh
go install github.com/caanmert/ai-session-tool/cmd/ais@latest
```

Requires Go 1.24+. Prebuilt macOS binaries are published on each tagged release.

## Interactive browser

Run `ais` with no arguments in a terminal:

- the list shows every session, newest first; the pane below previews the selected one (your prompts, replies rendered as Markdown, tool calls)
- `/` filters as you type: words match the title, prompts, project, branch, id and model; `t:codex`, `p:api`, `b:main` and `is:live` narrow by tool, project, branch and running sessions
- `enter` resumes the session in its own project directory, `f` forks it, `n` starts a new one in the same project. When the agent exits you are back in the list
- `y` copies the resume command, `tab` scrolls the preview, `r` rescans, `?` lists every key, `q` quits

Piped (`ais | head`), it prints the recent list instead.

## Usage

```sh
ais                          # interactive browser (or the recent list when piped)
ais ls                       # newest first: id, tool, project, branch, age, messages, tokens, title
ais ls -p api --since 7d     # filter by project path and recency
ais ls --live                # only sessions whose agent is running right now
ais ls --json | jq .         # scriptable output

ais show 3f2a                # details + transcript (any unique id prefix works)
ais show 3f2a --tools        # include tool output
ais show 3f2a --info         # details only

ais resume 3f2a              # cd into the session's project and run `claude --resume <id>` / `codex resume <id>`
ais resume 3f2a --fork       # continue in a new session, keep the original (`--fork-session` / `codex fork`)
ais resume 3f2a --print      # print the command instead: cd '/path' && claude --resume …

ais doctor                   # where ais looks, how many sessions it found, parse warnings
```

On a terminal, output is colored. Each tool has its own color, recent sessions stand out, and `show` draws a session card with replies rendered as Markdown (code blocks, lists, emphasis). Piped or redirected output stays plain text, so `grep`, `awk` and `--json` scripts keep working. Use `--color=always|never|auto` to override; `NO_COLOR` is honored. Light and dark terminal backgrounds are detected; set `AIS_THEME=dark` or `light` if your terminal doesn't report its background.

Example:

```
   ID        TOOL    PROJECT  BRANCH         UPDATED  MSGS  TOKENS  TITLE
   aaaaaaaa  codex   api      feat/webhooks  21h      4     21.2k   Webhook retries
●  22222222  claude  web      main           2d       3     2k      Dark mode
   11111111  claude  api      feat/auth      3d       5     4.2k    Fix auth token refresh
```

`●` marks a session whose agent is running right now (Claude Code only for now).

## How it works

`ais` only **reads** the tools' own files. It never modifies them.

| Tool | Transcripts | Running sessions | Override |
| --- | --- | --- | --- |
| Claude Code | `~/.claude/projects/<project>/<session-id>.jsonl` | `~/.claude/sessions/<pid>.json` | `CLAUDE_CONFIG_DIR` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl[.zst]` | not yet | `CODEX_HOME` |

For each transcript, `ais` collects:
- the working directory, git branch, model and version;
- prompt and reply counts;
- token usage, counting each API response once;
- a title: the session's own name (Claude's custom title, Codex's `session_index.jsonl`), else Claude's summary, else your first prompt.

Codex specifics:
- Rollouts older than a week are zstd-compressed (`.jsonl.zst`); `ais` reads both, preferring the plain file when both exist.
- A reverted thread gets a new rollout file that points at the earlier one through `history_base`; `ais` lists the newest file and stitches the history back together.
- Subagent and internal threads are hidden; archived threads (`archived_sessions/`) are not listed.

Lines it can't parse are skipped and reported by `ais doctor`.

Resuming runs the agent **in the session's original directory**, because `claude --resume` only finds sessions of the current project. On macOS and Linux, `ais` replaces itself with the agent process, so the agent gets the terminal exactly as if you had started it yourself.

## Roadmap

1. [x] Scaffold: Go module, Cobra CLI, lint, CI, goreleaser
2. [x] Claude Code adapter: `ls`, `show`, `resume`, `doctor`
3. [x] Codex adapter
4. [ ] SQLite index (incremental) + full-text `ais search`
5. [x] Bubble Tea TUI: list + preview, filter, resume / fork / new
6. [ ] Tags, rename, pin, archive/trash + restore
7. [ ] Token/cost stats by day, project, model
8. [ ] Parallel runner: tmux windows + git worktrees, "waiting for input" notifications

## Development

```sh
go test ./...                         # unit + golden tests on fixtures in testdata/
go test ./internal/cli -update        # accept changed golden output
golangci-lint run
go run ./cmd/ais doctor               # try it against your real sessions
```

Test fixtures under `testdata/` are made-up transcripts in each tool's real on-disk format. The Codex format was taken from the open-source [openai/codex](https://github.com/openai/codex) (`codex-rs/rollout`, `codex-rs/protocol`).
