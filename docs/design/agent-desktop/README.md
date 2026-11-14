# ais agent-desktop design handoff

A design for the ais desktop app: one place to run and manage Claude Code, Codex
and, later, other coding agents. This folder is a **reference**: the HTML here is
a static mockup, not production markup. Rebuild the screens in the existing
React frontend (`desktop/frontend/src`) using its components and `themes.css`.

Source canvas (editable): https://claude.ai/artifact/MztMuRdhxVqt4UdaRTSZsn

## What's here

| File | What it is |
| --- | --- |
| `workspace.html`, `workspace-light.html` | Agent workspace: sessions sidebar, conversation, changes panel |
| `board.html`, `board-light.html` | Agents board: every task across agents, grouped by state |
| `settings.html`, `settings-light.html` | Settings › Agents: connected agents, adding agents, routing, permissions |
| `workspace-960.html`, `board-960.html` (+ `-light`) | The same two screens at the 960×640 minimum window |
| `board-states.html` (+ `-light`) | Board problem states, status legend, refresh behavior |
| `empty-states.html` (+ `-light`) | First run, agent can't run, empty project, empty board |
| `dialogs.html` (+ `-light`) | Commit, stop and quit confirmations |
| `screenshots/` | A PNG of every file above, at 2× (same names; main screens end in `-1440`) |
| `tokens.css` | Light and dark theme tokens, using the variable names already in `themes.css` |

Open the HTML files in a browser to inspect exact spacing and colors. They load
Inter and JetBrains Mono from Google Fonts for the preview only. The app must
not load remote fonts (see Rules below).

## Rules from this repo that override the mockups

Where the mockup and these rules disagree, the rules win.

1. **Both themes.** Every screen is drawn in light and dark. Ship
   System/Light/Dark through the existing `Appearance.tsx`. Use only the CSS
   variables from `tokens.css`, never hard-coded colors, and check every new
   surface in both themes (dialogs, errors and approvals included).
2. **Window sizes.** Verify at 1360×900 (normal) and 960×640 (minimum). The
   main mockups are 1440 wide; the minimum window is drawn separately (see
   "Layout at narrow widths").
3. **No remote resources.** Bundle fonts locally or fall back to the system
   stack (`Inter, -apple-system, …` and `ui-monospace`). Render provider output
   without active HTML or remote images.
4. **Approvals fail closed.** "Always allow in this repo" must write the
   provider's own approval setting, not an ais-side override. Unsupported
   approval requests are denied, never auto-allowed.
5. **Nothing integrates automatically.** Commit and Open PR are explicit user
   actions with a confirmation that shows what will happen (see
   `dialogs.html`). No auto-commit, push or merge.
6. **Opening saved work never starts a model turn.** Selecting a session in the
   sidebar or board only shows it.
7. **Preserve work.** Stop ends the response, not the edits. Closing a tab hides
   work. The UI must say this where it matters (stop button tooltip, close
   confirmation).
8. **Workflow.** One issue and one focused PR per change, branch
   `<n>-<short-kebab-slug>` from `main`. Run the checks listed in `CLAUDE.md`.

## Scope: what to build now and what to leave

| Area in the mockup | Status | Tracking |
| --- | --- | --- |
| Theme tokens, typography, sidebar, session list, conversation, tool steps, approval card, composer, tabs, diff panel | **Build now** | UI and appearance pass [#21](https://github.com/caanmert/ai-session-tool/issues/21) |
| Agents board: task states, changed files, alerts, diff review | **Build now**, monitoring and review first | Runs in desktop [#15](https://github.com/caanmert/ai-session-tool/issues/15) |
| Usage limit bars, token and cost figures | Only where real data exists | Pricing [PR #10](https://github.com/caanmert/ai-session-tool/pull/10) / [#7](https://github.com/caanmert/ai-session-tool/issues/7) |
| Settings › Agents for the two current agents (status, CLI path, defaults) | Later, needs an issue | — |
| Hand off to another agent, Compare / second opinion, routing rules | Future, needs issues and design review | — |
| Add agent (Gemini CLI, Aider, OpenCode, custom) | Future, needs an agent adapter design first | — |
| Checkpoints with Restore | Future: restoring must never discard work silently | — |
| Launch / stop from the board | After monitoring; show effects and preserve task work | #15 |

Leave future items out of the UI entirely until their issue exists. Do not
render dead buttons.

## Design language

- **Look.** Neutral and quiet: graphite in dark, warm off-white in light, thin
  borders, one primary button per area (near-white in dark, near-black in
  light). Color is reserved for meaning: which agent, and what needs attention.
- **Theme differences beyond swapping values.** Agent, warning, success and
  danger colors each have a separate `-text` token, darker in light mode,
  because the bright shades fail contrast as text on white. Use `--warning` for
  icons and borders and `--warning-text` for words. The live terminal stays
  dark in both themes.
- **Agent identity.** Claude Code is orange with a **square** dot; Codex is blue
  with a **round** dot. The shape difference keeps them distinguishable without
  color. Keep the existing `AgentMark` component and restyle it with the
  `--agent-*` tokens.
- **Status.** Always text plus an icon or shape, never color alone:
  - Needs you (approval or question): `--warning`, alert icon. Highest priority anywhere it appears.
  - Running: neutral text with elapsed time; a thin progress bar in the agent's color only on board cards.
  - Queued: outlined square, muted.
  - Done: check icon, muted.
  - Failed or stopped: `--danger` text with the reason.
  - Agent waiting alerts stay visually distinct from process state (roadmap requirement).
- **Type.** Inter, base 14px, line-height 1.45. Titles 22–24px/600 with
  −0.015em tracking. Section labels 11px uppercase, 0.06em tracking, `--muted`.
  Code, paths, branches, commands and diff stats use `--font-mono` at 12–13px.
- **Spacing.** 4/8/12/16/22/28px steps. Content padding 28px; card padding 14px;
  gaps between cards 10–14px.
- **Shape.** Radii from `tokens.css`. 1px borders everywhere; no shadows except
  dialogs.
- **Icons.** Stroke icons through the existing `Icon` component (add the
  missing names: bell, pencil/edit, file, alert, stop, handoff, code, settings,
  filter, paperclip, arrow-up).
- **Focus.** Keep the existing `:focus-visible` outline using `--focus`.

## Components and where they live

| Component | Mockup | Existing code to change |
| --- | --- | --- |
| App shell (top bar + 3 columns) | all screens | `App.tsx` `.app-shell`, `style.css` |
| Top bar: workspace switcher, ⌘K search, agent usage pill, notifications | all | new, replaces parts of `.workspace-header` / `.toolbar` |
| Sidebar: New task, nav, Agents list, sessions grouped by project | workspace, board | `App.tsx` `.sidebar` (`.project-list`, `.agent-status`) |
| Session row: title, agent dot, status text | sidebar | `App.tsx` `.session-row` |
| Session header: breadcrumb, title, agent chip, status chip, meta, actions | workspace | `App.tsx` `.conversation-header`, `ChatWorkspace.tsx` |
| Tabs: Conversation / Plan / Terminal / Changes (count) | workspace | `WorkspaceTabs.tsx` |
| Message: avatar, author · time, markdown body | workspace | `ChatWorkspace.tsx`, `.message-heading`, `.markdown` |
| Tool-step group (Read / Search / Edit / Run rows, expandable) | workspace | `.tool-message` details in `App.tsx` / `ChatWorkspace.tsx` |
| Inline diff (line numbers, red/green rows) | workspace | `DiffViewer.tsx` |
| Command output block | workspace | `ChatWorkspace.tsx`; live output stays in `LiveTerminal.tsx` |
| Approval card: command, risk note, Allow once / Always allow / Deny with shortcuts | workspace, board | the existing approval/question UI in `ChatWorkspace.tsx` |
| Composer: textarea, agent picker, mode segmented control, attach, context %, send | workspace | `ChatWorkspace.tsx`, `ModelPicker.tsx`, `useChatDraft.ts` |
| Changes panel: file list with A/M and +/−, Review all | workspace | `DiffViewer.tsx` + new panel |
| Board columns: Needs you / Running / Queued / Done today | board | `RunDashboard.tsx`, `runs.css` |
| Task card | board | `RunDashboard.tsx` |
| Agent capacity card (connection, slots, usage bar) | board | new; data from the runner and pricing work |
| Settings page with sub-nav | settings | new; `Appearance.tsx` moves into Settings › Appearance |
| Switch (role="switch" button), radio cards, selects | settings | new shared components |

## Screen notes

### 1. Agent workspace (`workspace.html`)

- **Columns.** Sidebar 240px, conversation flexible (content max-width 860px),
  details panel 300px. Borders between columns, no gaps.
- **Header.** Mono breadcrumb `project / branch · worktree`; title; chips for
  agent and status; meta `Step n · elapsed · tokens`. Actions on the right:
  Open in editor and Stop (icon button, needs `aria-label` and a tooltip saying
  edits are kept). "Hand off" is future.
- **Conversation.** User and agent messages share one column. Agent tool calls
  are grouped into one bordered block with one row per step. A row shows the
  verb (muted), the target (mono) and a right-aligned summary. Edit rows expand
  to an inline diff; the most recent edit is expanded by default.
- **Approval card.** Warning border at 45% opacity and a 6% warning tint. Title
  names the agent. The command sits in a mono block, with a plain-language risk
  note on the right. Buttons: Allow once (primary, ⏎), Always allow in this repo
  (⌘⏎), Deny (Esc). Shortcuts are active only while the card has focus. Questions
  use the same card with a reply field instead of the command block.
- **Composer.** 14px radius. Shows the current agent and the permission mode
  (Plan / Auto-edit / Full auto: only the modes the provider supports). Keep
  draft recovery behavior.
- **Details panel.** Changes (file list, totals, Review all), then Session
  (context bar in the agent color, elapsed, cost only if known). Commit and Open
  PR only if in scope; see Rules 5.

### 2. Agents board (`board.html`)

- **Header.** Title plus a one-line summary ("4 tasks running · 2 need you · 9
  done today"). Board / List view toggle, agent filter, New task.
- **Capacity row.** One card per connected agent: connection status, running and
  waiting counts, slots used, usage bar with reset time. Hide the bar if usage
  data is unavailable; don't show 0%.
- **Columns, left to right:** Needs you, Running, Queued, Done today. Needs you
  comes first on purpose. Its cards allow approve/deny or reply in place, using
  the same approval rules as the workspace.
- **Task card.** Agent label and elapsed time; title links to the session;
  current step in mono; thin progress bar only when the runner reports real
  progress, otherwise omit it.
- **Problem states** (missing worktree, tmux unavailable, stopped, partial
  failure, exited): see "Board problem states" below.

### 3. Settings › Agents (`settings.html`)

- **Layout.** Sub-nav replaces the sidebar content; "Back to workspace" at the
  top. Content max-width 880px.
- **Agent card.** Mark, name, connection status, how it's signed in, detected CLI
  path; Test connection; enable switch. Defaults row: model (from
  `ModelPicker`'s data), mode, max parallel tasks.
- **Default permissions.** Three radio cards; "Always ask before these commands"
  shows a list of command patterns as chips.
- **Leave out for now:** Add an agent, Routing.

## Layout at narrow widths (`workspace-960.html`, `board-960.html`)

- **Below 1200px** the details panel leaves the layout. A "Changes 7" toggle
  button appears in the session header (`aria-pressed`), and ⌘\ also toggles it.
- **The drawer** opens from the right, 340px wide, over a scrim (`--overlay`)
  that covers only the conversation; the sidebar stays usable. It has a title,
  a close button and the same sections as the panel. It closes on Esc, the
  close button or a scrim click. Focus moves into the drawer and returns to the
  toggle.
- **At 960×640** the session header compacts: title on one line with ellipsis,
  icon-only Open in editor and Stop, a shorter meta line, no Changes tab (the
  toggle replaces it). The approval card and composer stay fully visible
  without scrolling. The sidebar stays 240px.
- **The board at 960** keeps columns at a fixed 250px and scrolls them sideways
  in one focusable region with a fade on the right edge. Capacity cards collapse
  into one compact line per agent (dot, name, slots, bar, %).
- Nothing wraps below the content. (The 1440 mockups do if you shrink the
  browser; ignore that.)

## Board problem states (`board-states.html`)

Roadmap #15 calls these normal states. Each card names the problem in one line
and offers a recovery action. Nothing in this sheet deletes work.

- **tmux unavailable**: one dismissible banner above the board, not an error on
  every card. Runs keep updating; only live output and attach are off.
  Actions: How to install, Check again.
- **Missing worktree**: red card border, the expected path in mono, and a note
  that the conversation and last captured diff are kept. Actions: Locate
  folder…, Remove (removes it from the board only; confirm first if it would
  hide unreviewed changes).
- **Stopped run**: neutral, "Stopped by you", with what was kept. Actions:
  Review changes, Open session. No Resume until launch and stop are designed.
- **Partial inspection failure**: the card keeps its process state; an amber
  inline note says what failed and shows the last good result with its time
  ("as of 14:20"). Action: Retry.
- **Agent exited**: red card with the exit code and the last two lines of
  output. Actions: View full log, Open session.
- **Process state vs attention**: two separate chips. Process state (Queued,
  Running, Stopped, Done, Exited) is an outlined chip with an icon. Attention
  (Needs approval, Has a question, Needs attention) is a filled chip next to
  it. A card can show both; never merge them into one label.
- **Refresh**: show "Updated 14:22". Keep the selected task and file selected
  across refreshes, even when the task moves to another column. ⌘R refreshes.

## Empty and error states (`empty-states.html`)

- **First run, no agents connected**: replaces the workspace content. Lists the
  agents ais looked for, with Connect when the CLI was found and How to install
  when it wasn't. Says that nothing is sent until a task starts.
- **Agent can't run**: in the sidebar, the agent row shows "Signed out" (amber
  text) or "Not found" (red, with icon). In Settings the agent card turns red,
  lists where ais looked, and offers a path field, Use this path and Check
  again. Signed out shows an amber notice with Sign in; running tasks finish,
  new ones wait.
- **Project with no sessions**: one line in the sidebar and a centered message
  with New task and Browse history.
- **Board with nothing running**: columns stay in place with dashed "Nothing
  here" boxes, so the layout doesn't jump.
- **Loading**: keep the existing `.loading-preview` pattern.

## Confirmation dialogs (`dialogs.html`)

Only for actions that change or end work. Each says what happens and what is
kept. Use a native `<dialog>` (or equivalent) with a focus trap, `aria-modal`,
a labelled title, Esc to cancel, and focus returned to the trigger.

- **Commit**: branch, an editable message drafted by the agent, file list with
  checkboxes (all checked), totals, and "Creates a local commit only. Nothing
  is pushed or merged." Primary: "Commit N files" (⌘⏎). Enter alone must not
  submit while the message field has focus.
- **Stop a running agent**: says the response ends and edits stay, with the
  kept file count. Buttons: Keep running (default focus), Stop response
  (`--danger-button`).
- **Quit with agents running**: the number of running agents, then what is kept
  (worktrees and edits, drafts, conversations). "Don't ask again when nothing
  is running" only affects the idle case. Buttons: Cancel (default focus), Quit
  and stop agents (`--danger-button`).
- Destructive buttons never get default focus.

## Suggested order of PRs

1. Tokens and theme: merge both theme blocks of `tokens.css` into
   `themes.css`, replace hard-coded colors in `style.css` and `runs.css` with
   variables, restyle `AgentMark`, add icons. Compare against the light and
   dark screenshots. (#21)
2. App shell, sidebar and session rows. (#21)
3. Session header, tabs, tool-step groups, inline diff. (#21)
4. Approval and question card, composer. (#21)
5. Details panel, the narrow-width drawer and the 960×640 header. (#21)
6. Empty and error states for the workspace and sidebar, and the stop and quit
   dialogs. (#21)
7. Board: monitoring and review, including every problem state, the status
   legend and refresh behavior. (#15)
8. Commit dialog, only if committing from the app is in scope (needs an issue).
9. Settings › Agents for the current two agents (new issue).
