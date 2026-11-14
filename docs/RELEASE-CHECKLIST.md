# Release validation

Use this checklist on the exact reviewed commit before tagging or publishing.
An artifact build or a browser fixture check alone does not establish native
window or live-provider compatibility. This checklist does not publish anything.

## Repeatable automated checks

Run the root and desktop checks in [CLAUDE.md](../CLAUDE.md), including building
the frontend before testing the Go module that embeds it. Do not run a frontend
build concurrently with desktop Go checks: Vite replaces embedded files.

```sh
python3 -m unittest discover -s scripts -v
goreleaser check
goreleaser release --snapshot --clean
python3 scripts/smoke_cli.py --dist dist --fixtures testdata
```

The smoke script verifies checksums and required regular files in all four
archives, then installs this host's binary into a temporary directory. It checks
version, Bash/Zsh/Fish completions, read-only setup, fixture session discovery,
and annotation/data retention after reinstall. It does not launch an agent,
change shell configuration, or install into a user's executable path. The Linux
packaging job runs the same smoke check against its packaged binary. Separate
install jobs download those exact snapshot archives and run them on macOS and
Linux, each on ARM64 and AMD64. The jobs use fresh hosted runners, require all
three completion shells, and assert the runner's actual OS/architecture.
Completion
syntax is checked when the corresponding shell is installed; missing shells are
reported explicitly.

The runner labels follow the [GitHub-hosted runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
Passing these jobs establishes archive installation on the tested runner images;
it does not test a published Homebrew tap or a desktop window.

On macOS, download the desktop artifact from the exact CI run and verify it:

```sh
gh run download <run-id> --repo caanmert/ai-session-tool \
  -n ais-desktop-macos-arm64 -D /tmp/ais-release-artifact
python3 scripts/verify_desktop.py \
  /tmp/ais-release-artifact/ais-desktop-macos-arm64.zip
```

This extracts into a temporary directory and checks archive paths/links, bundle
identity, executable permissions, architecture, the advertised and binary macOS
minimum, and the signature. It prints a SHA-256 for the downloaded ZIP. A valid
ad-hoc signature establishes bundle integrity, not Developer ID signing or
notarization. CI verifies its extracted ZIP before uploading it.

## Native window checks with fake providers

Prepare a source-built app and a local fixture subprocess; this uses no provider
account. Build the frontend/app first as described in [DESKTOP.md](DESKTOP.md).

```sh
go -C desktop test -c -o /tmp/ais-fixture-agent
python3 scripts/prepare_native_fixture.py \
  --app desktop/build/bin/ais-desktop.app \
  --agent /tmp/ais-fixture-agent \
  --output /tmp/ais-native-check
```

Choose a fresh output directory. The script creates a temporary Git project,
separate ais/Claude/Codex data directories, fake `claude` and `codex` wrappers,
and a separately identified, ad-hoc-signed **AIS Fixture.app**. It does not launch
the app. Open the prepared app and verify its project path is the fixture
directory before testing. Keep the ordinary installed app closed during this
check to avoid confusing the windows.

The fixture mode file defaults to `normal`. Change it to `approve` to display
a fixture approval, `deny` to test denial, or `hold` to keep a response running
for stop/quit checks. These subprocesses never execute provider turns or the
commands shown in their approval cards. Claude review requests automatically use
a fixed structured review response for `app.txt`.

- [ ] Select/browse folders, cancel the picker, and handle an unavailable folder.
- [ ] Switch History, saved work, conversations and parallel runs with mouse and keyboard.
- [ ] Resize to 960×640 and normal size; inspect chat, composer, dialogs and diffs.
- [ ] Change System/Light/Dark appearance, restart, and verify stored preferences.
- [ ] Type a multiline draft; close/reopen its tab and quit/restart without sending.
- [ ] Send a fixture response, select a model/effort, and inspect tool/approval controls.
- [ ] Allow and decline fixture approvals; stop a held response and continue it.
- [ ] Cancel quit with a held response, then confirm quit and verify restart recovery.
- [ ] Capture a review, inspect findings/diff links, return to the original conversation,
  and confirm stale-snapshot and already-sent-finding rejection.
- [ ] Launch/monitor/stop a disposable run with fake agents; confirm worktrees remain.

Record the app commit, OS/architecture, fixture modes, observed results and any
failures. Browser/native mocks do not replace these checks. For real providers,
use only the opt-in checks in DESKTOP.md when separately authorized; record exact
versions and usage scope. The current user requested fixture-only validation.

## Evidence from this development pass (2026-11-14)

| Check | Result |
| --- | --- |
| Root/desktop vet and race tests, frontend tests/builds | Passed; combined PRs #22/#23/#24 pass 54 frontend tests and desktop vet/race checks |
| Both-theme browser chat/history/review/library checks | Passed, including text contrast, errors/approvals, theme persistence and both window sizes |
| Dashboard browser checks | Passed in both themes: selection/refresh, attachment clipboard, missing worktree, stop confirmation, multiline launch, text contrast and minimum layout |
| Python packaging/desktop-verifier tests | 9 passed |
| GoReleaser 2.20.2 configuration and snapshot | Passed; four archives built without publishing |
| macOS ARM64 packaged CLI smoke | Passed: 8 fixture sessions, completion generation for three shells, Bash/Zsh syntax, retained pin and data after reinstall; Fish interpreter unavailable |
| Downloaded UI PR #22 artifact, CI run 43569006757 | Passed signature, executable permissions, ARM64 and binary/advertised macOS 13.0 checks |
| Downloaded ZIP SHA-256 | `21bcecb5b4f9428151c85e71623bd4d8ac0a926d776e8b88f02b23597cbd6428` |
| Native fixture preparation | Passed; fake-agent app built/prepared and signature verified |
| Native-window interactions | **Unverified:** computer-use connection timed out opening the fixture app; no interaction was claimed successful |
| Paid/live provider turns | **Not run, as requested** |
| Linux AMD64 packaged CLI smoke | Passed in [CI run 43572374305](https://github.com/caanmert/ai-session-tool/actions/runs/43572374305): 8 fixture sessions, Bash syntax, retained pin/data; Zsh/Fish interpreters unavailable |
| Four-platform CLI runtime matrix with all completion shells | Added; record actual CI results before claiming these platform checks passed |
| First Homebrew publication, Developer ID signing/notarization | Pending external publication/credentials; not performed |

Re-run relevant checks after source, dependencies, packaging, or provider versions
change. A reinstall smoke is not a complete real-version upgrade/downgrade test.
Complete the native checklist and clean-platform installation checks before
claiming release readiness. Publishing still follows [RELEASING.md](RELEASING.md).
