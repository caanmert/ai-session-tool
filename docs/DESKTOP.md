# ais desktop

A shared graphical workspace for Claude Code and Codex. Browse existing sessions,
continue them with streaming chat, and ask the other agent to review a captured
Git diff. Replies, tool activity, questions and approvals appear directly in the
app. The desktop app shares session readers, project rules, search and annotations
with the `ais` CLI.

## Download a CI build

Every CI run on `main` and on pull requests builds the app and keeps it as the
`ais-desktop-macos-arm64` artifact. It runs on macOS 13 or newer on Apple silicon
and doesn't need any of the build tools below.

To download the build from the latest successful run on `main` with the GitHub CLI:

```sh
run=$(gh run list --repo caanmert/ai-session-tool --workflow CI --branch main --status success --limit 1 --json databaseId -q '.[0].databaseId')
gh run download "$run" --repo caanmert/ai-session-tool -n ais-desktop-macos-arm64
ditto -x -k ais-desktop-macos-arm64.zip .
open ais-desktop.app
```

Without the GitHub CLI, open the repository's **Actions** tab, choose the latest
successful **CI** run on `main`, and download **ais-desktop-macos-arm64** under
**Artifacts**. You need to be signed in to GitHub. The download is a zip that
contains `ais-desktop-macos-arm64.zip`; unzip both to get `ais-desktop.app`, then
move it to Applications.

CI builds are ad-hoc signed, not signed with a Developer ID or notarized. Apps
downloaded with `gh` open normally. macOS quarantines browser downloads, so the
first time you open one it says Apple could not verify the app. On macOS 15 or
later, open **System Settings → Privacy & Security** and click **Open Anyway**. On
macOS 13 and 14, Control-click the app and choose **Open**. Only do this for builds
from this repository's CI.

## Build and open

This version targets macOS 13 or newer. Building requires Go 1.25+, Node.js 22.12+
with npm, and Xcode Command Line Tools. Go 1.24.7 remains sufficient for the CLI.
Desktop dependencies live in the separate `desktop/go.mod` module.

Install the pinned Wails tool:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
export PATH="$(go env GOPATH)/bin:$PATH"
```

From the repository root:

```sh
sh scripts/build-desktop.sh
open desktop/build/bin/ais-desktop.app
```

The script installs locked frontend dependencies and packages a native app for
your Mac's architecture. The resulting app needs neither Go nor Node to run;
you can move it to Applications. Install and sign in to Codex or Claude Code
separately. Only the provider you use is needed for chat; cross-provider review
requires both. Saved-history browsing works without either executable installed.

Local and CI builds use ad-hoc signing. A Developer ID signed, notarized release
is not yet published. The Homebrew CLI package does not install the desktop app.

## Screenshots

Choose **System**, **Light**, or **Dark** under **Appearance** in the sidebar.
System follows macOS appearance changes; an explicit choice stays active until
you change it. The app remembers the preference in its WebView storage and applies
it before the workspace loads. If the preference cannot be saved, it still
applies for the current visit and the sidebar explains that limitation.

These show the real React interface with fictional conversations and review data.
They demonstrate the layout, not output from a live model run.

![Graphical conversation with expandable tool activity](screenshots/desktop-chat.png)

![The workspace in dark appearance](screenshots/desktop-chat-dark.png)

![Preview the exact diff before sending it to the other agent](screenshots/desktop-review-preview.png)

![Review findings linked to files and ready to send back](screenshots/desktop-review.png)

## Start or continue a conversation

1. Select a project or **All sessions**. Search titles, project paths, branches,
   tags and conversation contents; use the provider filters to narrow results.
2. Select a saved conversation to preview it. **Continue in app** opens that
   provider's saved session in its recorded directory.
3. For a fresh conversation, choose **New session**, select a project and provider,
   then **Start in app**. Use **Browse** to choose another folder. The app remembers
   the twelve most recently added project folders.
4. Type in the message box. **Enter** sends; **Shift+Enter** inserts a new line.
   A new provider process starts when you send the first message. Responses stream
   into message bubbles, and expandable tool cards show commands and their output.
5. Answer questions or choose **Allow once** / **Decline** on approval cards.
   Approvals retain the installed agent's normal permission behavior; ais does
   not bypass permissions. Unsupported requests fail closed and may require the CLI.

Switch tabs or return to **History** while an agent works. **⌘K** opens history
search, **⌘N** opens New session, and **Esc** closes the new-session dialog.
History refreshes when a response finishes; the refresh button rescans manually.

Tabs show conversation titles, agent/project context, and working, waiting or
error states. **←/→**, **Home**, and **End** move between tabs when a tab has
keyboard focus. Close a stopped or idle tab with its close button; saved work
and drafts remain available. Busy tabs ask you to stop the response first.

When you scroll back through a conversation, new messages keep your reading
position. **Jump to latest** returns to the newest reply and resumes following.
Tool activity stays collapsed until opened. The composer and its model settings
remain available while reading longer replies.

Use **Pinned** or **Live** above history to narrow the list; their counts follow
the current project, agent, and search. Choose **Pinned · Recent**, **Oldest first**,
or **Title A–Z** to sort. Recent order keeps pinned conversations first. **Clear
filters** resets search, agent, and status while keeping the selected project.
The search clear button (or **Esc** in search) clears just the query.

Press **↓** in search to enter the list. **↑/↓**, **Home**, and **End** select and
preview conversations; **Enter** continues the focused conversation in the app.
Tab moves to the other controls. Pinned conversations can be managed with
`ais pin <id>` and `ais unpin <id>` in the CLI.

![Session history with status filters and sorting](screenshots/desktop-history.png)

**Stop response** terminates that response's local process group. Files already
changed remain changed; stopping is not an undo. Send a new message to continue
using the provider's saved session. Up to eight responses can run concurrently,
with up to 100 open tabs. Continuing an already-open native session selects the
existing conversation instead of creating another copy.

Unsent messages are saved on this Mac as you type, separately for each desktop
conversation. Closing a tab or restarting the app keeps the draft. It is cleared
after a successful send or when you choose **Discard draft**; a failed send keeps
the text. Drafts use the app WebView's local storage, separate from provider
transcripts and `desktop-chats` records. Clearing the app's website data removes
them. If storage is unavailable or full, the composer tells you to keep a copy;
you can still write and send messages.

Open tabs and review metadata persist across restarts. Closing a tab hides it
and retains its local record. Normal provider conversations can be reopened from
History; a review can also reopen its original conversation.

Choose **Saved workspace** in the sidebar to find desktop conversations and independent reviews, including closed tabs and fresh
conversations that have not sent a message yet. Filter by project or type, or
search titles, project paths and agents. The list includes review completion,
finding/sent counts and file counts.

**Reopen** restores the same tab and its saved state; **View** selects an already
open tab. Neither starts an agent or consumes model usage. Reviews retain their
captured diff and findings, including sent markers, even if their project folder
has moved or is temporarily unavailable. Sending messages or requesting fixes
still requires an available project and the normal snapshot checks. Opening a
review's **Original conversation** link also saves that tab's reopened state.
Use **Esc** or the close button to leave the library. Native provider sessions
remain available from History as before.

![Saved desktop conversations and reviews](screenshots/desktop-saved-workspace.png)

Quitting asks for confirmation if an agent is running. **Keep working** cancels
the quit; **Stop and quit** stops the local processes. Running responses do not
survive an app restart. An interrupted saved tab shows a stopped state, with no
stale approval buttons. Existing external terminal processes are not attached;
avoid simultaneously continuing the same native session in two applications.

## Choose a model and reasoning effort

The composer has **Model** and **Reasoning** selectors. Model names and supported
effort levels come from the installed agent; they are not a fixed list in ais.
Choose a model, then an effort level if that model supports it. Changing models
resets the effort override. **Agent default** leaves the choice to the provider's
settings; explicitly returning Codex to default resolves its configured model and
supported default effort, so a prior ais selection is not silently retained.

Choices are saved per conversation and applied to the next message, including
when continuing the same native session. Controls are disabled while a response
is running and while a setting is being saved. Existing conversations without
saved model preferences keep the provider's inherited behavior.

In the review preview, **Reviewer model** and **Reasoning** are separate choices.
They are remembered on the original conversation for future reviews. Sending
findings back uses the original conversation's coding model preferences.

A reply's model label comes from the agent's reported model, including reported
reroutes; it is not copied from the selection menu. Older imported messages or
provider events without model metadata may have no label. Completed reviews
show their reported model in the review banner.

**Refresh** reloads the catalog. Lists are otherwise cached for five minutes,
scoped by provider and project; reviewers use the same empty-directory scope as
review execution. Discovery initializes the CLI and requests metadata without
sending a prompt. Account restrictions still apply: a catalog entry is not proof
that a request will be authorized. Provider failures appear in the conversation.
An unavailable saved model stays visible until you select a replacement; ais does
not silently replace your saved choice.

If discovery fails, the picker shows the error and retains an **Agent default**
option. You can clear overrides or retry discovery after fixing the CLI. Models
with no advertised effort levels expose only the default effort. Cost estimates
and account usage displays are not part of these controls.

## Review with the other agent

The diff preview shows the selected file and added/removed line counts.
Use **↑/↓**, **Home**, and **End** in the file list to switch files, or click a
finding's file/line link to focus its location. **Wrap lines** makes long patches
easier to read. At larger window sizes, the **Files** slider adjusts the file
panel width. The code area is keyboard focusable for scrolling.

For example: implement with Codex, review with Claude Code, then return selected
findings to Codex for investigation and fixes. The reverse direction also works.

1. Finish or stop the original response, then click **Review with Claude Code**
   or **Review with Codex** in its header.
2. Inspect the file-by-file diff preview. The scope is **all current uncommitted
   changes relative to HEAD** in the Git working tree: staged, unstaged and eligible
   untracked files. This may include your own edits and changes from other tools.
   It is not limited to edits attributed to the selected conversation.
3. Click **Send snapshot**. This sends the displayed patch to the other provider
   using its installed account and can consume that provider's usage allowance.
   A separate review tab appears. The reviewer receives a fixed snapshot and is
   instructed to analyze it without editing or running commands. It runs from an
   empty temporary directory; Codex uses its read-only sandbox and Claude runs
   with tools, MCP servers and slash commands disabled.
4. Read the summary and findings. Click a file/line link to see its location in
   the captured diff. Findings are suggestions, not proof that an issue exists.
5. Select findings and click **Send findings**. The original conversation receives
   a request to verify them, fix valid issues and run relevant checks. Its normal
   approval flow remains active. This does not apply a patch automatically.

The app checks the snapshot again before starting the review and before sending
findings back. If the working tree changed, capture a fresh review. A preview is
single-use and expires after ten minutes. Already-sent findings cannot be sent
again through the same review.

Review requires a Git repository with an initial commit. Each snapshot is limited
to 100 changed files and 512 KiB of diff text. Binary contents and non-regular
untracked files are omitted with warnings; ignored files are excluded. Large
changes must be narrowed first. Findings must reference files and visible lines
in the snapshot, or use line zero for a file-level observation. Invalid reviewer
output is shown as an error and cannot be handed back as accepted findings.

## Parallel runs

Choose **Parallel runs** in the sidebar to monitor saved runs from the CLI or
desktop. Filter by project, select a run and task, then choose **All changes** or
a changed file. The dashboard shows the pane's process state, exit code, current
branch, worktree path, changed files and separate event/silence alerts. Missing
worktrees are reported per task; Git review remains available when tmux cannot
be inspected. Status refreshes every five seconds while the dashboard is open,
preserving task and file selection. **Refresh runs** also reloads saved manifests.

**Copy attach command** copies a tmux command for the selected task when its
pane is available. Paste it into a new terminal outside an existing tmux session
to answer questions or approvals in the agent's normal UI. The ais CLI is not
required. Copying does not execute the command or start another agent.

Use **New parallel run** to choose a project and agent and enter independent
task prompts. Each task gets a branch, worktree and tmux window at the same
committed HEAD. Uncommitted changes are not copied. The launch action starts
real local agents with their normal settings and approvals; it requires Git,
tmux and the chosen agent on PATH. There is no automatic commit or merge.
If setup fails partway through, saved manifests and work remain discoverable.

**Stop run…** opens a confirmation before terminating that run's tmux session.
Branches, worktrees and saved changes remain. A daemon-backed provider may
continue after its terminal exits; pane state and alerts do not prove it has
finished or is still waiting. Stop or pause agents before treating a diff as
final. A stopped run remains reviewable. Patch display is limited to 512 KiB;
select a file or use `ais run diff <id>` for the complete output.

![Desktop parallel run status and file review](screenshots/desktop-runs-light.png)

The fixture-only browser check is reproducible with:

```sh
AIS_PLAYWRIGHT_MODULE=/tmp/ais-ui-check/node_modules/playwright/index.mjs \
  node scripts/run-screenshots.mjs
```

It checks task/file selection, refresh, missing worktrees, explicit stop
confirmation, multiline task launch and the minimum window size, without
launching a provider or tmux. `AIS_SCREENSHOT_DIR` chooses an output directory.

## Agent discovery and local data

Finder does not load shell startup files. The app checks its inherited `PATH`
plus `~/.local/bin`, `/opt/homebrew/bin`, `/usr/local/bin` and standard system
locations. For custom installations, launch from a configured terminal:

```sh
./desktop/build/bin/ais-desktop.app/Contents/MacOS/ais-desktop
```

The app honors `CODEX_HOME`, `CODEX_SQLITE_HOME`, `CLAUDE_CONFIG_DIR`,
`AIS_CACHE_DIR` and `AIS_DATA_DIR`. Provider processes inherit your environment and
use their own sign-in. Recent projects live in `desktop-projects.json` under the
ais data directory. Desktop conversations and captured review patches are stored
in `desktop-chats/*.json` in that directory, using owner-only file permissions.
These records can contain source code, prompts and tool output. Provider-native
transcripts remain in the provider's own store.

The history preview contains up to 200 recent messages, each capped at 20,000 bytes.
Chat display retains up to 500 messages / 2 MiB of message text, with individual
messages capped at 100,000 bytes. A notice indicates shortened history. Embedded
HTML and remote images are not activated; links render as text. The provider's
native saved conversation is unaffected by display limits.

If startup fails, check installation and sign-in in the agent's CLI. `ais setup`
and `ais doctor` provide provider diagnostics. Restore moved or missing project
folders before continuing their sessions. **Reconnect** retries the desktop state
connection; it does not restart a failed model response. An initialization timeout
or unsupported protocol usually means the CLI needs attention or an update.

Git worktrees and nested repositories remain separate projects. Without Git,
projects match their exact directory. Archived native sessions are hidden. CLI
renames, pins and tags appear after refreshing.

## Development and validation

```sh
cd desktop/frontend
npm ci
npm test
npm run build
cd ..
go vet ./...
go test -race ./...
wails dev
```

Build the frontend before ordinary Go tests because the app embeds `frontend/dist`.
`wails dev` starts Go and Vite together; opening Vite alone does not provide the
native bridge. `frontend/src/api.ts` defines the typed `window.go.main.App` API.
Production builds skip unused generated bindings; Go and frontend API types must
be updated together.

Codex uses the [app-server protocol](https://learn.chatgpt.com/docs/app-server).
Claude Code uses its bidirectional stream-JSON CLI interface, including the
[streaming output](https://code.claude.com/docs/en/agent-sdk/streaming-output) and
[user-input control flow](https://code.claude.com/docs/en/agent-sdk/user-input).
Each response starts a provider process, resumes its native conversation ID where
available, and reaps the process on completion or stop. CLI compatibility is
version-sensitive. On October 12, 2026, authenticated backend smoke checks passed
with Codex 0.162.1 and Claude Code 2.1.296: new/resumed conversations, explicit
model selection, saved settings and conversation recovery after restart,
stop/continue, command approval denial and acceptance, and both directions of the
review-to-fix loop. Each fix was checked independently in a temporary repository.
The checks also caught and fixed an early-stop bug: a new Claude conversation no
longer retains an unusable session ID when stopped before its first model activity.

Backend tests use real local fixture subprocesses for both providers to exercise
streaming, native resume IDs, approval decisions, duplicate requests, malformed
output, stopping, persistence and process cleanup. Temporary Git repositories
exercise diff capture, review validation, cross-provider handoff and stale-snapshot
rejection. Frontend tests cover chat, approvals, review previews, finding selection,
late responses and history workflows. Model-control tests cover catalog pagination,
unsupported choices, defaults, preference persistence, resumed sessions, provider
model reporting, save failures and separate reviewer preferences. The earlier PTY implementation and its tests
remain available internally; the main UI now uses structured chat.

### Live compatibility checks

Ordinary `go test` skips authenticated checks. To repeat them from `desktop/`,
install and sign in to both CLIs, ensure `node` and `git` are on your PATH, then run:

```sh
AIS_TEST_INSTALLED_MODELS=1 go test -run '^TestInstalledModelCatalogs$' -v -count=1
AIS_TEST_LIVE_AGENTS=1 go test -run '^TestLive' -v -count=1 -timeout=15m
```

The first command queries catalogs without model turns. The second consumes real
model usage and creates native provider sessions. It uses temporary projects and
temporary app state, stops long responses as soon as output begins, and approves
only a fixed `printf` command or narrowly scoped fixture edits/test commands.
Provider-native session history may remain after the temporary projects are removed.
Unexpected approval requests fail the check without being granted. Select one
check/provider with Go's subtest filter, for example
`-run '^TestLiveStopFirstTurn$/claude$'`.

These are backend integration checks, not native window automation. Before release,
also manually check folder selection, tab switching, resizing, quit confirmation,
and the rendered live chat, approval and review controls in the macOS app.

### Reproduce the screenshots

With Google Chrome installed, start `npm run dev -- --port 5183` in
`desktop/frontend`. In another terminal, install Playwright into a temporary
folder and run the fixture-based visual check from the repository root:

```sh
npm install --prefix /tmp/ais-ui-check --no-audit --no-fund playwright@1.58.2
AIS_PLAYWRIGHT_MODULE=/tmp/ais-ui-check/node_modules/playwright/index.mjs \
  node scripts/desktop-screenshots.mjs
```

This replaces the desktop screenshots with fresh renders, checks a
960×640 viewport, and writes a small-window capture into the system temporary
directory. It injects fictional provider responses into a fresh browser context;
it does not read real conversations or contact an AI provider.

Repeat with `AIS_APPEARANCE=dark` to capture the dark screenshots and run the same
checks in that appearance. The script checks real rendered text contrast,
appearance persistence across reload, system appearance changes, approval/error
states, and the minimum window size. `AIS_SCREENSHOT_DIR` selects a different
output folder for validation without replacing documentation images.

## Current scope

The desktop app provides history browsing, graphical conversations and a
cross-provider review/fix workflow and parallel-run dashboard on macOS. GUI editing
of tags and titles, attachment support, full provider slash-command
parity, Windows/Linux support, and signed releases remain
future work. The CLI retains its session-management and parallel-task workflows.
