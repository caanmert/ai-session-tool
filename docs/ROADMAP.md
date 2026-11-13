# Desktop product roadmap

ais should make it easy to return to work, keep context between tools, and
understand what agents changed. The first priority is a dependable daily macOS
workspace. The CLI's session and parallel-task workflows remain supported.

[Roadmap issue #11](https://github.com/caanmert/ai-session-tool/issues/11) groups
the work. The [project board](https://github.com/users/caanmert/projects/5) is the
source of truth for delivery status. An open PR means the work is ready for
review, not that it has shipped. This plan sets an order, not deadlines.

## First: returning to everyday work

| Work | Outcome | Tracking |
| --- | --- | --- |
| History navigation | Find pinned or live sessions, sort deliberately, and preview/continue with the keyboard | [#12](https://github.com/caanmert/ai-session-tool/issues/12), [PR #17](https://github.com/caanmert/ai-session-tool/pull/17) |
| Draft recovery | Keep unsent requests across tab closure and app restarts; retain text after a failed send | [#13](https://github.com/caanmert/ai-session-tool/issues/13), [PR #18](https://github.com/caanmert/ai-session-tool/pull/18) |
| Saved workspace | Reopen conversations and independent reviews without starting an agent; retain findings and captured diffs | [#14](https://github.com/caanmert/ai-session-tool/issues/14), [PR #19](https://github.com/caanmert/ai-session-tool/pull/19) |

Draft recovery and the saved library form one user journey: a new conversation
has no native transcript until a provider runs, so its saved desktop record must
remain discoverable. Validate closing and reopening through the library as well
as app restarts. A reopened review must keep its original-conversation link and
the checks that reject stale findings.

The separate [pricing PR #10](https://github.com/caanmert/ai-session-tool/pull/10)
implements [issue #7](https://github.com/caanmert/ai-session-tool/issues/7). Its
request-level usage and cache migration changes should be checked with the
desktop module as well as the CLI. Do not create a second implementation of the
same issue.

## Next: parallel work in the desktop app

[#15](https://github.com/caanmert/ai-session-tool/issues/15) brings saved runs,
task state, alerts, changed files, and diff review into the graphical workspace.
Build it on the existing runner rather than duplicating Git/tmux behavior.
Missing worktrees, unavailable tmux, partial inspection failures, and stopped
runs are normal states to design for. Refresh must preserve task and file
selection. Agent waiting alerts must remain distinguishable from process state.

Start with monitoring and review. Refine launch and stop interactions before
adding them; show their effects and preserve task work. Integration remains a
deliberate user action, with no automatic commit or merge.

## Then: release confidence

[#16](https://github.com/caanmert/ai-session-tool/issues/16) covers clean
installation, upgrades, data preservation, artifact verification, and publication
readiness. The existing CI artifact is useful for testing; it does not replace a
signed, notarized desktop release or the pending first CLI/Homebrew publication.
See [RELEASING.md](RELEASING.md) for the external publishing prerequisites.

Before release, record evidence for:

- Native macOS folder selection, keyboard shortcuts, tab switching, resizing,
  quit confirmation, and app restart, including drafts and native dialogs.
- The rendered chat, questions/approvals, review preview, findings, original
  conversation navigation, and stale-snapshot rejection.
- Downloaded app executable permissions and signature, first launch, and the
  documented macOS/architecture requirement.
- CLI package contents, shell completions, clean install, upgrade, and user-data
  preservation on each platform claimed as tested.
- Installed provider compatibility, with exact CLI versions. Authenticated
  checks consume model usage and require explicit authorization; fixture tests
  and a successful browser render do not establish live compatibility.

## Design and engineering agreements

Preserve work first. Saved conversations, review snapshots, drafts, annotations,
and task worktrees have different lifetimes from the rebuildable search index.
Closing a tab hides work; stopping a response does not undo file edits.

Use a consistent, readable interface with clear selected states, visible keyboard
focus, concise status text, and useful empty/error recovery. Verify the minimum
960×640 window as well as the normal 1360×900 layout. Prefer native controls
where they provide reliable keyboard and accessibility behavior.

Keep data local by default and disclose what is sent when invoking a provider.
Opening saved work must not start a model turn. Render provider output without
active HTML or remote images. Preserve installed provider approvals and fail
closed on unsupported approval requests.

Keep the Go/frontend API typed on both sides. Return compact metadata for lists,
and fetch full content only when opening a record. Use fixture subprocesses for
protocol regressions, temporary Git repositories for review behavior, and user
workflow tests for the frontend. Avoid tests that only repeat implementation
details.

Use one issue and focused PR per change, targeting `main`. Move planned work
through Todo → In Progress → In Review → Done. Run the checks in [CLAUDE.md](../CLAUDE.md),
inspect CI, and test related PRs together. Keep issues open until merge and
acceptance criteria are satisfied. Merging, publishing, and deployment require
authorization; a green check alone does not authorize them.
