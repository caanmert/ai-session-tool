#!/usr/bin/env python3
"""Capture the real ais TUI with disposable demo data, then render PNGs."""

import argparse
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import time

from PIL import Image, ImageDraw, ImageFont
import pyte

ROOT = Path(__file__).resolve().parent.parent
COLS, ROWS = 116, 30


def write(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def jsonl(path, rows):
    write(path, "".join(json.dumps(row) + "\n" for row in rows))


def sessions(root):
    now = datetime.now(timezone.utc)
    titles = [
        ("codex", "Webhook retries with backoff", "atlas-api", "feat/retries"),
        ("claude", "Fix token refresh before expiry", "atlas-api", "fix/auth"),
        ("codex", "Keyboard navigation for checkout", "storefront", "feat/a11y"),
        ("claude", "Document the installation flow", "docs-site", "docs/install"),
        ("codex", "Add coverage for CSV exports", "analytics", "test/exports"),
        ("claude", "Reduce dashboard loading time", "storefront", "perf/dashboard"),
    ]
    names = []
    for n, (tool, title, project, branch) in enumerate(titles):
        sid = f"{n+1:08d}-1111-4111-8111-111111111111"
        stamp = (now - timedelta(minutes=6 + n * 23)).isoformat()
        cwd = "/demo/" + project
        prompt = "Add retries to the webhook sender using exponential backoff.\nKeep retries bounded and respect context cancellation." if n == 0 else title + "."
        answer = "Implemented bounded retries in `internal/webhook/send.go`.\n\n- Retry transient failures up to **5 attempts**.\n- Back off between attempts; stop when the context is cancelled.\n- Log the attempt number and delay.\n\nValidation: `go test ./internal/webhook/...` passes." if n == 0 else "Updated the implementation and added regression coverage. All targeted checks pass."
        if tool == "codex":
            rows = [
                {"timestamp": stamp, "type": "session_meta", "payload": {"id": sid, "timestamp": stamp, "cwd": cwd, "source": "cli", "git": {"branch": branch}}},
                {"timestamp": stamp, "type": "turn_context", "payload": {"model": "gpt-5.5-codex", "turn_id": sid}},
                {"timestamp": stamp, "type": "event_msg", "payload": {"type": "user_message", "message": prompt}},
                {"timestamp": stamp, "type": "event_msg", "payload": {"type": "agent_message", "message": answer}},
                {"timestamp": stamp, "type": "event_msg", "payload": {"type": "token_count", "info": {"total_token_usage": {"input_tokens": 12000 + n * 1300, "cached_input_tokens": 7000, "output_tokens": 1800, "total_tokens": 13800 + n * 1300}}}},
            ]
            jsonl(root / "codex" / "sessions" / f"rollout-{stamp[:19].replace(':', '-')}-{sid}.jsonl", rows)
            names.append({"id": sid, "thread_name": title})
        else:
            base = {"cwd": cwd, "sessionId": sid, "gitBranch": branch, "timestamp": stamp}
            rows = [
                {"type": "summary", "summary": title},
                dict(base, type="user", message={"role": "user", "content": prompt}),
                dict(base, type="assistant", message={"id": "message-" + sid, "role": "assistant", "model": "claude-sonnet-4-5", "content": [{"type": "text", "text": answer}], "usage": {"input_tokens": 4500 + n * 500, "output_tokens": 1800}}),
            ]
            jsonl(root / "claude" / "projects" / project / f"{sid}.jsonl", rows)
    jsonl(root / "codex" / "session_index.jsonl", names)


def render(capture, output, title, font_path):
    screen = pyte.Screen(COLS, ROWS)
    pyte.Stream(screen).feed(capture.rstrip("\n").replace("\n", "\r\n"))
    font = ImageFont.truetype(font_path, 18)
    cell, line = round(font.getlength("M")), 26
    pad, top = 26, 66
    width, height = COLS * cell + pad * 2, ROWS * line + top + 24
    image = Image.new("RGB", (width, height), "#10141c")
    draw = ImageDraw.Draw(image)
    draw.rectangle((0, 0, width, 44), fill="#1c2230")
    for x, color in zip((26, 46, 66), ("#ef6b73", "#e6be6b", "#89c997")):
        draw.ellipse((x-5, 17, x+5, 27), fill=color)
    draw.text((94, 10), title, font=font, fill="#c7d1e0")
    label = "DEMO DATA"
    draw.text((width - pad - font.getlength(label), 10), label, font=font, fill="#8290a6")
    palette = {"black": "#10141c", "red": "#ef6b73", "green": "#89c997", "brown": "#e6be6b", "blue": "#7faaf0", "magenta": "#ba9cf0", "cyan": "#78c9d2", "white": "#d5deeb", "brightblack": "#8793a7", "brightwhite": "#ffffff"}

    def color(value, default):
        if value == "default":
            return default
        return palette.get(value, "#" + value if len(value) == 6 else default)

    for y in range(ROWS):
        for x in range(COLS):
            char = screen.buffer[y][x]
            fg, bg = color(char.fg, "#d5deeb"), color(char.bg, "#10141c")
            if char.reverse:
                fg, bg = bg, fg
            left, baseline = pad + x * cell, top + y * line
            draw.rectangle((left, baseline, left + cell, baseline + line), fill=bg)
            draw.text((left, baseline), char.data, font=font, fill=fg, stroke_width=0)
            if char.underscore:
                draw.line((left, baseline + 22, left + cell, baseline + 22), fill=fg)
    output.parent.mkdir(parents=True, exist_ok=True)
    image.save(output, optimize=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=ROOT / "ais")
    parser.add_argument("--tmux", default=shutil.which("tmux"))
    parser.add_argument("--font", default="/System/Library/Fonts/Menlo.ttc")
    args = parser.parse_args()
    if not args.tmux:
        parser.error("tmux is required; install it or pass --tmux /path/to/tmux")
    binary = str(args.binary.resolve())
    with tempfile.TemporaryDirectory(prefix="ais-demo-", dir="/tmp") as tmp:
        root = Path(tmp)
        env = dict(os.environ, AIS_DATA_DIR=str(root / "data"), AIS_CACHE_DIR=str(root / "cache"), AIS_CONFIG=str(root / "config.toml"), CODEX_HOME=str(root / "codex"), CLAUDE_CONFIG_DIR=str(root / "claude"), AIS_THEME="dark", TERM="xterm-256color", COLORTERM="truecolor", GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1")
        for name in ("TMUX", "TMUX_PANE", "NO_COLOR", "CODEX_SQLITE_HOME"):
            env.pop(name, None)
        socket = str(root / "tmux.sock")
        wrapper = root / "bin" / "tmux"
        write(wrapper, "#!/bin/sh\nexec " + shlex.join([args.tmux, "-S", socket]) + ' "$@"\n')
        wrapper.chmod(0o755)
        env["PATH"] = str(wrapper.parent) + os.pathsep + env["PATH"]

        def run(argv, **kwargs):
            return subprocess.check_output(argv, env=env, text=True, stderr=subprocess.STDOUT, **kwargs).rstrip("\n")

        def tmux(*argv):
            return run([args.tmux, "-S", socket, *argv])

        def launch(name, command):
            cmd = shlex.join(["env", *[key + "=" + env[key] for key in ("PATH", "AIS_DATA_DIR", "AIS_CACHE_DIR", "AIS_CONFIG", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "AIS_THEME", "COLORTERM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM")], binary, "--color=always", *command])
            tmux("new-session", "-d", "-s", name, "-x", str(COLS), "-y", str(ROWS), cmd)
            tmux("set-option", "-t", name, "status", "off")
            tmux("resize-window", "-t", name, "-x", str(COLS), "-y", str(ROWS))

        def capture(name, marker):
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                text = tmux("capture-pane", "-p", "-e", "-t", name)
                if marker in text and "loading…" not in text and "scanning sessions" not in text:
                    return text + "\n"
                time.sleep(0.2)
            raise RuntimeError(f"Screen {name} did not become ready:\n{text}")

        try:
            sessions(root)
            run([binary, "pin", "00000001"])
            run([binary, "tag", "00000001", "backend", "reliability"])
            launch("browser", [])
            browser = capture("browser", "Implemented bounded retries")
            render(browser, ROOT / "docs/screenshots/sessions.png", "ais  /  find and resume sessions", args.font)

            repo = root / "atlas-api"
            repo.mkdir()
            def git(*argv):
                return run(["git", "-C", str(repo), "-c", "user.name=Demo", "-c", "user.email=demo@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", *argv])
            git("init", "-b", "main")
            before = 'package webhook\n\nimport "time"\n\nconst maxAttempts = 1\n\nfunc retryDelay(attempt int) time.Duration {\n\treturn time.Second\n}\n'
            write(repo / "internal/webhook/retry.go", before)
            write(repo / "README.md", "# Atlas API\n\nA demo webhook service.\n")
            git("add", ".")
            git("commit", "-m", "Demo baseline")
            base = git("rev-parse", "HEAD")
            tasks = []
            for n, prompt in enumerate(("Add bounded webhook retries", "Cover retry cancellation", "Document retry behavior"), 1):
                name, branch = f"task-{n}", f"ais-demo/task-{n}"
                worktree = root / name
                git("worktree", "add", "-b", branch, str(worktree), base)
                tasks.append({"prompt": prompt, "branch": branch, "path": str(worktree), "window": name})
                # These are placeholder processes, never real coding agents.
                command = "new-session" if n == 1 else "new-window"
                target = ["-s", "ais-demo"] if n == 1 else ["-t", "ais-demo:"]
                pane = tmux(command, "-d", *target, "-n", name, "-P", "-F", "#{pane_id}", "sleep 300")
                tmux("set-option", "-w", "-t", pane, "automatic-rename", "off")
                tmux("set-option", "-w", "-t", pane, "remain-on-exit", "on")
                tmux("set-option", "-p", "-t", pane, "@ais_task", name)
                if n != 2:
                    tmux("respawn-pane", "-k", "-t", pane, "true")
            after = before.replace("maxAttempts = 1", "maxAttempts = 5").replace("return time.Second", "if attempt > maxAttempts {\n\t\tattempt = maxAttempts\n\t}\n\treturn time.Second * (1 << attempt)")
            write(root / "task-1/internal/webhook/retry.go", after)
            write(root / "task-1/internal/webhook/retry_test.go", "package webhook\n\n// Demo regression tests would go here.\n")
            write(root / "task-2/internal/webhook/cancel_test.go", "package webhook\n\n// Demo cancellation test.\n")
            write(root / "task-3/README.md", "# Atlas API\n\nRetries stop after five attempts or context cancellation.\n")
            manifest = {"id": "ais-demo", "tool": "codex", "repository": str(repo), "base": base, "createdAt": datetime.now(timezone.utc).isoformat(), "tasks": tasks}
            write(root / "data/runs/ais-demo/run.json", json.dumps(manifest))
            launch("review", ["run", "review", "ais-demo", "--refresh", "0s"])
            capture("review", "maxAttempts")
            tmux("send-keys", "-t", "review", "Tab", "Down", "Enter")
            time.sleep(0.3)
            review = capture("review", "maxAttempts")
            render(review, ROOT / "docs/screenshots/run-review.png", "ais  /  review parallel tasks", args.font)
            print("Saved docs/screenshots/sessions.png and run-review.png")
        finally:
            subprocess.run([args.tmux, "-S", socket, "kill-server"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


if __name__ == "__main__":
    main()
