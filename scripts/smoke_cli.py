#!/usr/bin/env python3
"""Check all release archives and smoke-install this host's CLI using fixtures only."""
import argparse
import json
import os
from pathlib import Path, PurePosixPath
import platform
import shutil
import subprocess
import tarfile
import tempfile

from homebrew import render


def smoke(dist, fixtures):
    dist, fixtures = Path(dist).resolve(), Path(fixtures).resolve()
    version = json.loads((dist / "metadata.json").read_text())["version"]
    # The formula generator verifies all four archives against checksums.txt.
    render("v" + version, dist)
    required = {"ais", "README.md", "docs/USAGE.md", "docs/RELEASING.md", "docs/RELEASE-CHECKLIST.md",
                "completions/ais.bash", "completions/ais.zsh", "completions/ais.fish"}
    for system in ["darwin", "linux"]:
        for arch in ["arm64", "amd64"]:
            with tarfile.open(dist / f"ais_{version}_{system}_{arch}.tar.gz") as archive:
                for member in archive.getmembers():
                    path = PurePosixPath(member.name)
                    if path.is_absolute() or ".." in path.parts:
                        raise ValueError("unsafe archive path: " + member.name)
                for name in required:
                    if not archive.getmember(name).isfile():
                        raise ValueError("missing regular file: " + name)
    system = platform.system().lower()
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(platform.machine().lower())
    if system not in ["darwin", "linux"] or not arch:
        raise ValueError("runtime smoke check supports macOS/Linux ARM64/AMD64")
    with tempfile.TemporaryDirectory(prefix="ais-install-smoke-") as directory:
        root = Path(directory)
        binary = root / "bin/ais"
        binary.parent.mkdir()
        data = root / "data"
        data.mkdir()
        sentinel = data / "keep.txt"
        sentinel.write_text("retain local data")

        def install():
            with tarfile.open(dist / f"ais_{version}_{system}_{arch}.tar.gz") as archive:
                with archive.extractfile("ais") as source, binary.open("wb") as destination:
                    shutil.copyfileobj(source, destination)
            binary.chmod(0o755)

        env = dict(os.environ, AIS_DATA_DIR=str(data), AIS_CACHE_DIR=str(root / "cache"),
                   AIS_CONFIG=str(root / "config.toml"), CODEX_HOME=str(fixtures / "codex"),
                   CODEX_SQLITE_HOME=str(root / "codex-db"), CLAUDE_CONFIG_DIR=str(fixtures / "claude"))

        def run(*args):
            return subprocess.check_output([str(binary), *args], env=env, text=True)

        install()
        if version not in run("--version"):
            raise ValueError("installed binary version does not match its archive")
        run("setup")
        syntax_checks = {}
        for shell in ["bash", "zsh", "fish"]:
            completion = run("completion", shell)
            if "ais" not in completion:
                raise ValueError("empty completion output: " + shell)
            script = root / ("ais." + shell)
            script.write_text(completion)
            interpreter = shutil.which(shell)
            if interpreter:
                flags = {"bash": ["--noprofile", "--norc", "-n"], "zsh": ["-f", "-n"], "fish": ["--no-config", "-n"]}[shell]
                subprocess.run([interpreter, *flags, str(script)], check=True)
                syntax_checks[shell] = "passed"
            else:
                syntax_checks[shell] = "interpreter not installed"
        sessions = json.loads(run("ls", "--json"))
        if not sessions or {session["tool"] for session in sessions} != {"codex", "claude"}:
            raise ValueError("fixture session discovery failed")
        chosen = sessions[0]["id"]
        run("pin", chosen)
        install()
        restored = json.loads(run("ls", "--json"))
        if sentinel.read_text() != "retain local data" or not any(session["id"] == chosen and session.get("pinned") for session in restored):
            raise ValueError("reinstall lost local annotations or data")
    return {"version": version, "archives_verified": 4, "runtime_platform": system + "/" + arch,
            "fixture_sessions": len(sessions), "completion_shells": ["bash", "zsh", "fish"],
            "completion_syntax": syntax_checks,
            "reinstall_preserves_annotations_and_data": True, "provider_turns": 0}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=Path("dist"))
    parser.add_argument("--fixtures", type=Path, default=Path("testdata"))
    args = parser.parse_args()
    print(json.dumps(smoke(args.dist, args.fixtures), indent=2))
