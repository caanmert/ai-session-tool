# Screenshots

These PNGs show the running `ais` terminal interface with fictional transcripts
and disposable Git worktrees. The capture script uses a separate tmux server,
local demo data, and placeholder task processes. It captures terminal contents
and colors, renders them to PNG, and adds a small window title bar.

- `sessions.png`: browse Claude Code and Codex sessions across projects and
  preview a conversation, including tags and token usage.
- `run-review.png`: inspect task pane states, changed files, and a selected diff.

## Regenerate

Requires Go, Python 3, Git, tmux, and a monospace font. From the repository root:

```sh
go build -o ais ./cmd/ais
python3 -m venv /tmp/ais-screenshots-env
/tmp/ais-screenshots-env/bin/pip install -r scripts/screenshots-requirements.txt
/tmp/ais-screenshots-env/bin/python scripts/screenshots.py
```

The default font is macOS Menlo. On Linux, supply an installed monospace font:

```sh
/tmp/ais-screenshots-env/bin/python scripts/screenshots.py \
  --font /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf
```

Use `--tmux /path/to/tmux` or `--binary /path/to/ais` when needed. The script
replaces the two PNG files in this directory, cleans up its own tmux server and
temporary data, and leaves existing tmux sessions untouched. It does not start
real coding agents or use your personal transcripts.

After regenerating, inspect both images for readable text, correct colors, and
complete previews before committing. Session ages and temporary worktree paths
may vary between captures.
