#!/usr/bin/env python3
"""Prepare (but do not launch) an isolated native app with local fake agents."""
import argparse
import json
import os
from pathlib import Path
import plistlib
import shlex
import shutil
import subprocess
import sys
import tempfile
import uuid

from verify_desktop import check_info


def prepare(app, agent, output=None):
    if sys.platform != "darwin":
        raise ValueError("native fixture preparation requires macOS")
    app, agent = Path(app).resolve(), Path(agent).resolve()
    with (app / "Contents/Info.plist").open("rb") as stream:
        check_info(plistlib.load(stream))
    if not agent.is_file() or not os.access(agent, os.X_OK):
        raise ValueError("pass the executable built by go test -c")
    if output is not None:
        root = Path(output).resolve()
        root.mkdir(mode=0o700, parents=True, exist_ok=False)
    else:
        root = Path(tempfile.mkdtemp(prefix="ais-native-fixture-"))
    for name in ["bin", "data/desktop-chats", "cache", "codex/sessions", "codex-db", "claude/projects", "project"]:
        (root / name).mkdir(parents=True, exist_ok=True, mode=0o700)
    fixture = root / "agent-fixture"
    shutil.copy2(agent, fixture)
    project = root / "project"
    git_env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    git_env.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
                   GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
                   GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid")

    def git(*args):
        subprocess.run(["git", "-C", str(project), *args], env=git_env, check=True)

    git("init", "-q")
    (project / "app.txt").write_text("before\n")
    git("add", "app.txt")
    git("commit", "-qm", "Fixture base")
    (project / "app.txt").write_text("after\n")
    mode = root / "mode"
    mode.write_text("normal\n")
    for provider in ["codex", "claude"]:
        wrapper = root / "bin" / provider
        wrapper.write_text("#!/bin/sh\nmode=normal\nIFS= read -r mode < " + shlex.quote(str(mode)) +
                           "\ncase \"$*\" in *--json-schema*) mode=review;; esac\nexec " + shlex.quote(str(fixture)) +
                           " -test.run='^TestAgentProcess$' -- ais-fake " + provider + " \"$mode\" \"$@\"\n")
        wrapper.chmod(0o700)
    chat_id = str(uuid.uuid4())
    state = {"id": chat_id, "tool": "codex", "project": str(project), "sessionId": "", "sourceKey": "",
             "title": "Native fixture conversation", "status": "idle", "error": "", "messages": [], "approvals": [],
             "updatedAt": "2026-11-14T00:00:00Z", "version": 1, "closed": False, "shortened": False}
    record = root / "data/desktop-chats" / (chat_id + ".json")
    record.write_text(json.dumps(state))
    record.chmod(0o600)
    (root / "data/desktop-projects.json").write_text(json.dumps([str(project)]))
    destination = root / "AIS Fixture.app"
    subprocess.run(["ditto", str(app), str(destination)], check=True)
    info_path = destination / "Contents/Info.plist"
    with info_path.open("rb") as stream:
        info = plistlib.load(stream)
    info.update(CFBundleIdentifier="io.github.caanmert.ais.fixture." + uuid.uuid4().hex,
                CFBundleName="AIS Fixture", CFBundleDisplayName="AIS Fixture",
                LSEnvironment={"AIS_DATA_DIR": str(root / "data"), "AIS_CACHE_DIR": str(root / "cache"),
                               "CODEX_HOME": str(root / "codex"), "CODEX_SQLITE_HOME": str(root / "codex-db"),
                               "CLAUDE_CONFIG_DIR": str(root / "claude"),
                               "PATH": str(root / "bin") + ":/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"})
    with info_path.open("wb") as stream:
        plistlib.dump(info, stream)
    subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(destination)], check=True)
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(destination)], check=True)
    result = {"app": str(destination), "root": str(root), "chat_id": chat_id, "mode_file": str(mode), "provider_turns": 0}
    (root / "fixture.json").write_text(json.dumps(result, indent=2))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app", type=Path, required=True)
    parser.add_argument("--agent", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    print(json.dumps(prepare(args.app, args.agent, args.output), indent=2))
