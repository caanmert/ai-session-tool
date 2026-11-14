#!/usr/bin/env python3
"""Verify a downloaded/CI desktop ZIP on macOS, without launching the app."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import plistlib
import re
import stat
import subprocess
import sys
import tempfile
import zipfile


def check_archive_paths(archive):
    total = 0
    links = {}
    for entry in archive.infolist():
        path = PurePosixPath(entry.filename)
        if path.is_absolute() or ".." in path.parts or not path.parts:
            raise ValueError("unsafe ZIP path: " + entry.filename)
        total += entry.file_size
        if total > 256 * 1024 * 1024:
            raise ValueError("desktop ZIP exceeds the 256 MiB extraction limit")
        if stat.S_ISLNK(entry.external_attr >> 16):
            if entry.file_size > 4096:
                raise ValueError("oversized symlink")
            target = PurePosixPath(archive.read(entry).decode("utf-8"))
            if target.is_absolute():
                raise ValueError("absolute symlink: " + entry.filename)
            links[path] = target

    def resolve(parts, followed=()):
        result = []
        for index, part in enumerate(parts):
            if part == "..":
                if len(result) <= 1:
                    raise ValueError("symlink escapes its bundle")
                result.pop()
            elif part != ".":
                result.append(part)
            current = PurePosixPath(*result)
            if current in links:
                if current in followed or len(followed) >= 40:
                    raise ValueError("cyclic or excessive symlink chain")
                return resolve(result[:-1] + list(links[current].parts) + list(parts[index + 1:]), followed + (current,))
        return result

    for entry in archive.infolist():
        path = PurePosixPath(entry.filename)
        if tuple(resolve(path.parent.parts)) != path.parent.parts:
            raise ValueError("ZIP entry writes through a symlink: " + entry.filename)
        resolve(path.parts)


def check_info(info):
    executable = info.get("CFBundleExecutable", "")
    if not executable or PurePosixPath(executable).name != executable:
        raise ValueError("bundle executable must be a filename")
    if info.get("CFBundleIdentifier") != "io.github.caanmert.ais":
        raise ValueError("unexpected bundle identifier")
    if info.get("CFBundlePackageType") != "APPL":
        raise ValueError("bundle is not an app")
    if info.get("LSMinimumSystemVersion") != "13.0":
        raise ValueError("app must advertise the documented macOS 13.0 minimum")
    return executable


def command(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()


def verify(path, architecture="arm64"):
    if sys.platform != "darwin":
        raise ValueError("verify desktop artifacts on macOS (codesign/lipo/vtool required)")
    path = Path(path).resolve()
    with zipfile.ZipFile(path) as archive:
        check_archive_paths(archive)
    with tempfile.TemporaryDirectory(prefix="ais-artifact-") as directory:
        command("ditto", "-x", "-k", str(path), directory)
        app = Path(directory) / "ais-desktop.app"
        with (app / "Contents/Info.plist").open("rb") as stream:
            executable = check_info(plistlib.load(stream))
        binary = app / "Contents/MacOS" / executable
        if not binary.is_file() or not os.access(binary, os.X_OK):
            raise ValueError("executable permissions did not survive packaging")
        actual_arch = command("lipo", "-archs", str(binary)).split()
        if actual_arch != [architecture]:
            raise ValueError("unexpected app architectures: " + ", ".join(actual_arch))
        build = command("xcrun", "vtool", "-show-build", str(binary))
        versions = re.findall(r"\bminos\s+(\d+(?:\.\d+)+)", build)
        if len(versions) != 1 or tuple(map(int, versions[0].split(".")[:2])) > (13, 0):
            raise ValueError("binary minimum exceeds advertised macOS 13.0: " + build)
        command("codesign", "--verify", "--deep", "--strict", str(app))
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return {"archive": str(path), "sha256": digest.hexdigest(), "architecture": architecture,
            "minimum_macos": versions[0], "signature_valid": True, "executable_permissions": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--architecture", default="arm64", choices=["arm64", "x86_64"])
    args = parser.parse_args()
    try:
        print(json.dumps(verify(args.archive, args.architecture), indent=2))
    except (ValueError, OSError, subprocess.CalledProcessError, zipfile.BadZipFile) as error:
        parser.exit(1, str(error) + "\n")


if __name__ == "__main__":
    main()
