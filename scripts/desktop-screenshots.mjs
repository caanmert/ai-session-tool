// Run against the local Vite preview. All provider responses below are fictional.
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { mkdir } from "node:fs/promises";
import { contrastFailures } from "./ui-contrast.mjs";
const { chromium, webkit } = await import(
  process.env.AIS_PLAYWRIGHT_MODULE || "playwright"
);
const engine = process.env.AIS_BROWSER || "chrome";
if (!["chrome", "webkit"].includes(engine))
  throw Error("AIS_BROWSER must be chrome or webkit");
const browser = await (engine === "webkit"
  ? webkit.launch({ headless: true })
  : chromium.launch({ channel: "chrome", headless: true }));
try {
  const page = await browser.newPage({
    // Transitions are off so captures and contrast checks see settled frames.
    reducedMotion: "reduce",
    viewport: { width: 1360, height: 900 },
    deviceScaleFactor: 1,
  });
  const errors = [];
  // Captures taken straight after a resize can show the previous frame.
  async function resize(width, height) {
    await page.setViewportSize({ width, height });
    await page.evaluate(
      () =>
        new Promise((done) =>
          requestAnimationFrame(() => requestAnimationFrame(done)),
        ),
    );
  }
  page.on("pageerror", (e) => errors.push(String(e)));
  const mode = process.env.AIS_APPEARANCE || "system";
  const suffix = mode === "dark" ? "-dark" : "";
  await page.emulateMedia({ colorScheme: "light" });
  await page.addInitScript((mode) => {
    if (!localStorage.getItem("ais:appearance:v1"))
      localStorage.setItem("ais:appearance:v1", mode);
  }, mode);
  await page.addInitScript(() => {
    const project = "/projects/checkout";
    const source = {
      id: "chat",
      tool: "codex",
      project,
      comparison: "cmp-demo",
      // Continued with Claude Code: the link and the hand-off button show.
      handedTo: ["chat-claude-next"],
      // Near the window, so the amber indicator is checked for contrast.
      context: { used: 168000, window: 200000 },
      checkpoints: [
        { id: "p1", messageId: "u", commit: "abc", at: "", kind: "turn" },
      ],
      sessionId: "demo",
      sourceKey: "codex/demo",
      title: "Make checkout retries safe",
      status: "idle",
      error: "",
      approvals: [],
      updatedAt: "2026-10-04T10:00:00Z",
      shortened: false,
      version: 1,
      settings: { model: "codex-demo", effort: "high", configured: true },
      reviewerSettings: {
        model: "claude-demo",
        effort: "medium",
        configured: true,
      },
      activeModel: "codex-demo",
      messages: [
        {
          id: "u",
          role: "user",
          kind: "text",
          text: "Add safe retries to checkout requests. Keep the response clear when a payment fails.",
          at: new Date(Date.now() - 26 * 60_000).toISOString(),
        },
        {
          id: "a",
          role: "assistant",
          kind: "text",
          model: "codex-demo",
          text: "I added bounded retries and an idempotency key for each checkout attempt. The request now uses the same key across retries, so a timeout will not create a second charge.",
          at: new Date(Date.now() - 25 * 60_000).toISOString(),
        },
        {
          id: "r",
          role: "assistant",
          kind: "tool",
          name: "Read",
          status: "completed",
          text: '{\n  "file_path": "/projects/checkout/src/checkout.ts"\n}',
        },
        {
          id: "s",
          role: "assistant",
          kind: "tool",
          name: "Grep",
          status: "completed",
          text: '{\n  "pattern": "client.charge"\n}\n\nsrc/checkout.ts:2',
        },
        {
          id: "e",
          role: "assistant",
          kind: "tool",
          name: "Edit",
          status: "completed",
          text: '{\n  "file_path": "/projects/checkout/src/checkout.ts",\n  "old_string": "return client.charge(order);",\n  "new_string": "const key = crypto.randomUUID();\\nreturn retry(() => client.charge(order, { key }), 3);"\n}\n\nThe file was updated.',
        },
        {
          id: "t",
          role: "assistant",
          kind: "tool",
          name: "npm test — checkout",
          status: "completed",
          text: "✓ reuses the idempotency key\n✓ retries transient failures\n✓ returns the final response\n\n3 tests passed",
        },
        {
          id: "b",
          role: "assistant",
          kind: "text",
          model: "codex-demo",
          at: new Date(Date.now() - 22 * 60_000).toISOString(),
          text: "The focused tests pass. The changes are ready for a second review.\n\n- Reuse one request key across retries\n- Retry transient failures up to three times\n- Return the last response to the caller\n\n```ts\n// Retry transient failures only\nexport async function charge(order: Order) {\n  const key = crypto.randomUUID();\n  return retry(() => client.charge(order, { key }), 3);\n}\n```\n\n| Case | Result |\n| --- | --- |\n| Timeout | Retried with the same key |\n| Declined card | Returned at once |",
        },
      ],
    };
    const patch =
      "diff --git a/src/checkout.ts b/src/checkout.ts\n--- a/src/checkout.ts\n+++ b/src/checkout.ts\n@@ -1,4 +1,9 @@\n export async function checkout(order: Order) {\n-  return client.charge(order);\n+  const key = crypto.randomUUID();\n+  let response;\n+  for (let attempt = 0; attempt < 3; attempt++) {\n+    response = await client.charge(order, { key });\n+    if (response.ok) break;\n+  }\n+  return response;\n }\n";
    const snapshot = {
      root: project,
      base: "26ac85e309ce",
      hash: "b7ad519e026e502dca9a",
      diff: patch,
      files: [{ path: "src/checkout.ts", patch }],
      warnings: [],
    };
    const review = {
      ...source,
      id: "review",
      tool: "claude",
      handedTo: undefined,
      title: "Review with Claude Code",
      settings: source.reviewerSettings,
      activeModel: "claude-demo",
      messages: [],
      review: {
        sourceId: "chat",
        snapshot,
        summary:
          "The idempotency key is reused correctly. One retry condition needs attention before this is ready.",
        complete: true,
        findings: [
          {
            id: "f1",
            priority: "P2",
            file: "src/checkout.ts",
            line: 6,
            title: "Stop retrying permanent payment failures",
            body: "The loop retries every unsuccessful response, including declined cards and invalid payment details. Limit retries to transient failures; return permanent failures immediately so the user receives a useful response without extra requests.",
            sent: false,
          },
        ],
      },
    };
    const chats = { chat: source, review };
    window.__aisDemo = { source, review };
    const session = {
      key: "codex/demo",
      id: "demo",
      tool: "codex",
      title: source.title,
      cwd: project,
      project,
      gitBranch: "feat/checkout-retries",
      updatedAt: source.updatedAt,
      userTurns: 1,
      assistantTurns: 2,
      usage: { input: 1200, output: 450, cacheRead: 0, cacheWrite: 0 },
      pinned: true,
    };
    window.go = {
      main: {
        App: {
          SetAppearance: async () => {},
          ComparisonOverview: async () => ({
            id: "cmp-demo",
            project: "/projects/checkout",
            base: "a1b2c3d4e5f6",
            prompt: "Make checkout retries safe",
            sides: [
              {
                tool: "claude",
                chatId: "claude-side",
                worktree: "/data/worktrees/cmp-demo/checkout-claude",
                branch: "ais-compare/demo/claude",
                status: "idle",
                changes: {
                  available: true,
                  files: [
                    {
                      path: "src/checkout.ts",
                      status: "M",
                      added: 9,
                      removed: 1,
                    },
                    {
                      path: "src/retry.ts",
                      status: "?",
                      added: 31,
                      removed: 0,
                    },
                  ],
                  added: 40,
                  removed: 1,
                },
              },
              {
                tool: "codex",
                chatId: "chat",
                worktree: "/data/worktrees/cmp-demo/checkout-codex",
                branch: "ais-compare/demo/codex",
                status: "idle",
                changes: {
                  available: true,
                  files: [
                    {
                      path: "src/checkout.ts",
                      status: "M",
                      added: 7,
                      removed: 1,
                    },
                  ],
                  added: 7,
                  removed: 1,
                },
              },
            ],
          }),
          ComparisonDiff: async (_id, tool) => ({
            root: "/data",
            base: "a1b2c3d4e5f6",
            hash: tool,
            diff: "",
            warnings: [],
            files: [
              {
                path: "src/checkout.ts",
                patch:
                  tool === "claude"
                    ? "diff --git a/src/checkout.ts b/src/checkout.ts\n@@ -1,3 +1,4 @@\n export function checkout(order) {\n-  return charge(order);\n+  const key = crypto.randomUUID();\n+  return retry(() => charge(order, { key }), 3);\n }\n"
                    : "diff --git a/src/checkout.ts b/src/checkout.ts\n@@ -1,3 +1,3 @@\n export function checkout(order) {\n-  return charge(order);\n+  return withRetries(charge, order);\n }\n",
              },
            ],
          }),
          UsageLimits: async () => {
            const later = (hours) =>
              new Date(Date.now() + hours * 3_600_000).toISOString();
            return {
              agents: [
                {
                  tool: "claude",
                  plan: "Pro",
                  limits: [
                    {
                      label: "5-hour session",
                      window: "session",
                      percent: 59,
                      resetsAt: later(2.2),
                      level: "normal",
                    },
                    {
                      label: "Weekly · all models",
                      window: "weekly",
                      percent: 88,
                      resetsAt: later(8.3),
                      level: "warning",
                    },
                    {
                      label: "Weekly · Fable",
                      window: "weekly",
                      percent: 88,
                      resetsAt: later(8.3),
                      level: "warning",
                    },
                  ],
                  extraUsage: "off",
                  breakdown: [
                    { label: "Claude Code", percent: 95 },
                    { label: "Cowork", percent: 4 },
                    { label: "Chats", percent: 1 },
                  ],
                  readAt: new Date().toISOString(),
                },
                {
                  tool: "codex",
                  plan: "Plus",
                  limits: [
                    {
                      label: "Weekly",
                      window: "weekly",
                      percent: 37,
                      resetsAt: later(110),
                      level: "normal",
                    },
                  ],
                  resetCredits: { available: 3, nextExpiry: later(24 * 14) },
                  readAt: new Date().toISOString(),
                },
              ],
            };
          },
          AgentSkills: async (tool) => ({
            tool,
            skills:
              tool === "codex"
                ? [
                    {
                      name: "pdf:pdf",
                      description: "Read, create and check PDF files",
                      scope: "user",
                      path: "/s/pdf",
                    },
                    {
                      name: "review-agent",
                      description:
                        "Perform a read-only, defect-first review of a change",
                      scope: "system",
                      path: "/s/review",
                    },
                    {
                      name: "release-notes",
                      description:
                        "Draft release notes from the merged pull requests",
                      scope: "repo",
                      path: "/s/notes",
                    },
                  ]
                : [
                    {
                      name: "engineering:code-review",
                      description:
                        "Review code changes for security, performance and correctness",
                      hint: "[pr]",
                    },
                    {
                      name: "engineering:debug",
                      description: "Structured debugging session",
                    },
                  ],
          }),
          SetChatWebSearch: async () => {},
          GitHubIssue: async (_project, number) => ({
            number,
            title: "Retrying a declined card charges twice",
            body: "When a card is declined, checkout retries it three times. Each retry can charge the customer again.\n\n## Steps\n1. Use a card that is declined\n2. Pay\n\n## Done means\n- A decline returns at once\n- Timeouts retry with the same key",
            url: `https://github.com/acme/checkout/issues/${number}`,
            state: "OPEN",
            author: "ana",
            labels: [
              { name: "bug", color: "d73a4a" },
              { name: "payments", color: "0e8a16" },
            ],
            createdAt: new Date(Date.now() - 3 * 86_400_000).toISOString(),
            updatedAt: new Date(Date.now() - 2 * 3_600_000).toISOString(),
            comments: [
              {
                author: "bo",
                body: "Seen twice in production this week. The second charge is refunded by hand.",
                createdAt: new Date(Date.now() - 5 * 3_600_000).toISOString(),
              },
            ],
            commentCount: 1,
          }),
          GitHubPull: async (_project, number) => ({
            number,
            title: "Make checkout retries safe",
            body: "Retries reuse one idempotency key, so a timeout can't charge twice.\n\nCloses #42",
            url: `https://github.com/acme/checkout/pull/${number}`,
            state: "OPEN",
            author: "ana",
            labels: [],
            createdAt: new Date(Date.now() - 86_400_000).toISOString(),
            updatedAt: new Date(Date.now() - 3_600_000).toISOString(),
            comments: [],
            commentCount: 0,
            branch: "ais/make-checkout-retries-safe",
            base: "main",
            isDraft: false,
            review: "REVIEW_REQUIRED",
            checks: [
              { name: "test (macos-latest)", state: "passing" },
              { name: "lint", state: "passing" },
              { name: "desktop", state: "running" },
            ],
            additions: 40,
            deletions: 2,
            changedFiles: 3,
          }),
          SearchMessages: async () => [
            {
              kind: "chat",
              chatId: "chat",
              messageId: "a",
              title: "Make checkout retries safe",
              tool: "codex",
              snippet:
                "…an idempotency key for each \u0002checkout\u0003 attempt. The request now uses the same key…",
            },
          ],
          ProjectCommands: async () => [
            { label: "test", command: "npm run test", source: "package.json" },
            {
              label: "build",
              command: "npm run build",
              source: "package.json",
            },
            { label: "lint", command: "npm run lint", source: "package.json" },
          ],
          ProjectRun: async () => ({
            command: "npm run test",
            running: false,
            exitCode: 0,
            output:
              "> checkout@1.0.0 test\n> vitest run\n\n ✓ test/checkout.test.ts (6 tests) 41ms\n ✓ test/retry.test.ts (4 tests) 12ms\n\n Test Files  2 passed (2)\n      Tests  10 passed (10)\n",
            startedAt: "2026-11-17T10:00:00Z",
            endedAt: "2026-11-17T10:00:03Z",
          }),
          StartProjectCommand: async (_id, command) => ({
            command,
            running: true,
            exitCode: 0,
            output: "",
            startedAt: "2026-11-17T10:00:00Z",
          }),
          StopProjectCommand: async () => {},
          ProjectInstructions: async () => [
            {
              agent: "claude",
              scope: "project",
              name: "CLAUDE.md",
              path: "/projects/checkout/CLAUDE.md",
              exists: true,
              size: 1840,
              editable: true,
            },
            {
              agent: "claude",
              scope: "project",
              name: ".claude/CLAUDE.md",
              path: "/projects/checkout/.claude/CLAUDE.md",
              exists: false,
              size: 0,
              editable: true,
            },
            {
              agent: "claude",
              scope: "project",
              name: "CLAUDE.local.md",
              path: "/projects/checkout/CLAUDE.local.md",
              exists: false,
              size: 0,
              editable: true,
            },
            {
              agent: "codex",
              scope: "project",
              name: "AGENTS.md",
              path: "/projects/checkout/AGENTS.md",
              exists: true,
              size: 620,
              editable: true,
            },
            {
              agent: "claude",
              scope: "user",
              name: "~/.claude/CLAUDE.md",
              path: "/Users/me/.claude/CLAUDE.md",
              exists: true,
              size: 2400,
              editable: false,
            },
            {
              agent: "codex",
              scope: "user",
              name: "~/.codex/AGENTS.md",
              path: "/Users/me/.codex/AGENTS.md",
              exists: false,
              size: 0,
              editable: false,
            },
          ],
          ReadInstruction: async (_project, name) => ({
            name,
            text: "# Checkout\n\n- Run `npm test -- checkout` before calling a change done.\n- Never retry a declined card.\n- Keep the idempotency key for every retry.\n",
            exists: true,
            version: "v1",
          }),
          WriteInstruction: async (_project, name, text) => ({
            name,
            text,
            exists: true,
            version: "v2",
          }),
          RevealInstruction: async () => {},
          RestorePlan: async () => [
            { path: "src/retry.ts", change: "remove" },
            { path: "src/checkout.ts", change: "restore" },
            { path: "test/checkout.test.ts", change: "restore" },
          ],
          RestoreCheckpoint: async () => [],
          DiscardFile: async () => {},
          GitHubIssueForm: async () => ({
            repository: "acme/checkout",
            url: "https://github.com/acme/checkout",
            labels: [
              {
                name: "bug",
                color: "d73a4a",
                description: "Something isn't working",
              },
              { name: "payments", color: "0e8a16", description: "" },
              { name: "documentation", color: "0075ca", description: "" },
            ],
          }),
          GitHubOverview: async () => ({
            repository: "acme/checkout",
            url: "https://github.com/acme/checkout",
            issues: [
              {
                number: 42,
                title: "Retrying a declined card charges twice",
                url: "https://github.com/acme/checkout/issues/42",
                author: "ana",
                labels: ["bug", "payments"],
                labelColors: { bug: "d73a4a", payments: "0e8a16" },
                updatedAt: "2026-10-04T09:00:00Z",
              },
              {
                number: 39,
                title: "Document the retry contract for API clients",
                url: "https://github.com/acme/checkout/issues/39",
                author: "bo",
                labels: ["documentation"],
                labelColors: { documentation: "0075ca" },
                updatedAt: "2026-10-02T09:00:00Z",
              },
            ],
            pulls: [
              {
                number: 44,
                title: "Make checkout retries safe",
                url: "https://github.com/acme/checkout/pull/44",
                author: "ana",
                branch: "ais/make-checkout-retries-safe",
                isDraft: false,
                updatedAt: "2026-10-04T10:00:00Z",
                review: "REVIEW_REQUIRED",
                checks: "running",
              },
              {
                number: 41,
                title: "Bound retries for refunds",
                url: "https://github.com/acme/checkout/pull/41",
                author: "bo",
                branch: "refund-retries",
                isDraft: true,
                updatedAt: "2026-10-03T10:00:00Z",
                review: "CHANGES_REQUESTED",
                checks: "failing",
              },
            ],
          }),
          CommitPlan: async () => ({
            branch: "main",
            defaultBranch: "main",
            onDefault: true,
            files: [
              { path: "src/checkout.ts", status: "M", added: 7, removed: 1 },
              { path: "src/retry.ts", status: "?", added: 24, removed: 0 },
              {
                path: "test/checkout.test.ts",
                status: "M",
                added: 18,
                removed: 3,
              },
            ],
            added: 49,
            removed: 4,
            message: "Make checkout retries safe",
            newBranch: "ais/make-checkout-retries-safe",
          }),
          PullRequestPlan: async () => ({
            repository: "acme/checkout",
            remote: "origin",
            branch: "ais/make-checkout-retries-safe",
            base: "main",
            commits: ["Make checkout retries safe"],
            unpushed: 1,
            uncommitted: 0,
            title: "Make checkout retries safe",
            body: "Retries transient failures and keeps one request key.",
          }),
          ListRuns: async () => [
            {
              id: "ais-demo",
              tool: "claude",
              repository: "/projects/checkout",
              base: "a1b2c3d4e5f6",
              createdAt: "2026-11-14T08:30:00Z",
              notifications: true,
              tasks: [],
            },
          ],
          RunStatus: async () => ({
            id: "ais-demo",
            base: "a1b2c3d4e5f6",
            tasks: [
              {
                task: {
                  window: "task-1",
                  prompt: "Bound checkout retries and keep the request key",
                  branch: "ais-demo/task-1",
                  path: "/work/checkout/task-1",
                },
                state: "running",
                attention: true,
                quiet: false,
                files: [{ status: "M", path: "src/checkout.ts" }],
              },
              {
                task: {
                  window: "task-2",
                  prompt: "Test permanent payment failures",
                  branch: "ais-demo/task-2",
                  path: "/work/checkout/task-2",
                },
                state: "running",
                attention: false,
                quiet: false,
                files: [],
              },
              {
                task: {
                  window: "task-3",
                  prompt: "Document the retry contract",
                  branch: "ais-demo/task-3",
                  path: "/work/checkout/task-3",
                },
                state: "exited",
                exitCode: 0,
                attention: false,
                quiet: false,
                files: [
                  { status: "M", path: "README.md" },
                  { status: "A", path: "docs/retries.md" },
                ],
              },
            ],
          }),
          ChatModes: async () => [
            {
              id: "plan",
              label: "Plan",
              detail: "Reads and proposes a plan. Changes nothing.",
            },
            {
              id: "",
              label: "Default",
              detail:
                "Edits files in the project. Asks before commands that need more access.",
            },
          ],
          SetChatMode: async () => {},
          AgentHealth: async () => [
            {
              tool: "codex",
              version: "0.167.1",
              tested: "0.162.1",
            },
            {
              tool: "claude",
              version: "2.1.296",
              tested: "2.1.296",
            },
          ],
          ChatModels: async (_id, reviewer) => ({
            defaultModel: reviewer ? "claude-demo" : "codex-demo",
            defaultEffort: "medium",
            models: [
              {
                id: reviewer ? "claude-demo" : "codex-demo",
                name: reviewer ? "Claude demo model" : "Codex demo model",
                description: "Fictional model for UI documentation",
                efforts: ["low", "medium", "high"],
                defaultEffort: "medium",
              },
            ],
          }),
          SetChatSettings: async (id, settings, reviewer) => {
            chats[id][reviewer ? "reviewerSettings" : "settings"] = settings;
            chats[id].version++;
          },
          Refresh: async () => ({
            sessions: [
              session,
              {
                ...session,
                key: "claude/architecture",
                id: "architecture",
                tool: "claude",
                title: "Map the payment service boundaries",
                gitBranch: "main",
                pinned: false,
                live: { pid: 12345 },
                userTurns: 4,
                assistantTurns: 6,
              },
              {
                ...session,
                key: "codex/tests",
                id: "tests",
                pinned: false,
                title: "Add regression tests for order totals",
                gitBranch: "test/order-totals",
                updatedAt: "2026-10-03T08:30:00Z",
              },
            ],
            projects: [
              {
                path: project,
                name: "checkout",
                sessions: 3,
                available: true,
                recent: false,
              },
            ],
            tools: [
              { name: "codex", available: true, path: "/demo/codex" },
              { name: "claude", available: true, path: "/demo/claude" },
            ],
            warnings: [],
          }),
          Search: async () => ["codex/demo"],
          // A saved transcript, in the shape the CLI's reader produces.
          Transcript: async () => {
            const said = source.messages.filter((m) => m.kind === "text");
            const text = (m) => ({
              role: m.role,
              kind: "text",
              text: m.text,
              time: m.at,
            });
            const call = (toolName, value) => ({
              role: "assistant",
              kind: "tool_use",
              toolName,
              text: value,
            });
            const result = (value) => ({
              role: "user",
              kind: "tool_result",
              text: value,
            });
            return {
              messages: [
                text(said[0]),
                text(said[1]),
                call("Read", `${project}/src/checkout.ts`),
                call("Grep", "client.charge"),
                result("export async function checkout(order) {"),
                result("src/checkout.ts:2"),
                call("Edit", `${project}/src/checkout.ts`),
                result("The file was updated."),
                call("Bash", "npm test -- checkout"),
                result("3 tests passed"),
                text(said[2]),
              ],
              truncated: false,
            };
          },
          ListChats: async () => [source],
          ListSavedChats: async () =>
            Object.values(chats).map((chat) => ({
              id: chat.id,
              tool: chat.tool,
              project: chat.project,
              title: chat.title,
              status: chat.status,
              updatedAt: chat.updatedAt,
              closed: !!chat.closed,
              isReview: !!chat.review,
              complete: !!chat.review?.complete,
              findingCount: chat.review?.findings.length || 0,
              sentCount:
                chat.review?.findings.filter((f) => f.sent).length || 0,
              fileCount: chat.review?.snapshot.files.length || 0,
            })),
          ReopenChat: async (id) => {
            chats[id].closed = false;
            return chats[id];
          },
          ReadChat: async (id, version) => ({
            version: chats[id].version,
            state:
              version === chats[id].version ? null : structuredClone(chats[id]),
          }),
          ResumeChat: async () => source,
          NewChat: async () => source,
          PickProject: async () => "",
          ChatChanges: async () => ({
            available: true,
            files: [
              { path: "src/checkout.ts", status: "M", added: 7, removed: 1 },
              { path: "src/retry.ts", status: "?", added: 24, removed: 0 },
              {
                path: "test/checkout.test.ts",
                status: "M",
                added: 18,
                removed: 3,
              },
            ],
            added: 49,
            removed: 4,
          }),
          // A file too large to show is listed above the diff.
          ChatDiff: async () => ({
            ...snapshot,
            warnings: [
              "fixtures/orders.json is larger than 512 KiB and is not shown.",
            ],
            total: 2,
            partial: true,
          }),
          TurnDiff: async () => snapshot,
          CopyChanges: async () => snapshot,
          BringBackCopy: async () => ["src/checkout.ts"],
          RemoveCopy: async () => {},
          NewChatInCopy: async () => source,
          SavedPrompts: async () => [
            {
              id: "starter-review",
              name: "Review for problems",
              text: "Review the uncommitted changes in {project} for bugs, security problems and missing error handling.",
            },
            {
              id: "starter-tests",
              name: "Write tests for the changes",
              text: "Write tests for the changes on {branch}: {changed files}.",
            },
          ],
          SetSavedPrompts: async () => {},
          PrepareReview: async () => ({
            id: "draft",
            sourceId: "chat",
            tool: "claude",
            snapshot,
            created: source.updatedAt,
          }),
          StartReview: async () => review,
          SendReviewFindings: async () => {
            review.review.findings[0].sent = true;
            review.version++;
            return "chat";
          },
          SendChat: async () => {},
          StopChat: async () => {},
          CloseChat: async (id) => {
            chats[id].closed = true;
          },
          AnswerChat: async (id) => {
            chats[id].approvals = [];
            chats[id].status = "idle";
            chats[id].version++;
          },
        },
      },
    };
  });
  await page.goto(process.argv[2] || "http://127.0.0.1:5183");
  if (mode === "system") {
    await page.emulateMedia({ colorScheme: "dark" });
    await page.waitForFunction(
      () => document.documentElement.dataset.theme === "dark",
    );
    await page.emulateMedia({ colorScheme: "light" });
  }
  await page.waitForFunction(
    (mode) =>
      document.documentElement.dataset.theme ===
      (mode === "dark" ? "dark" : "light"),
    mode,
  );
  const opposite = mode === "dark" ? "light" : "dark";
  // Appearance lives in Settings and must persist across a reload.
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByLabel("Appearance", { exact: true }).selectOption(opposite);
  await page.reload();
  await page.waitForFunction(
    (theme) => document.documentElement.dataset.theme === theme,
    opposite,
  );
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByLabel("Appearance", { exact: true }).selectOption(mode);
  const screenshots =
    process.env.AIS_SCREENSHOT_DIR ||
    fileURLToPath(new URL("../docs/screenshots/", import.meta.url));
  await mkdir(screenshots, { recursive: true });
  const settingsContrast = await page.evaluate(contrastFailures);
  if (settingsContrast.length)
    throw Error(`Settings contrast: ${JSON.stringify(settingsContrast)}`);
  await page.screenshot({
    path: `${screenshots}/desktop-settings${suffix}.png`,
  });
  // Saved prompts, further down Settings.
  const savedPrompts = page.getByRole("region", { name: "Saved prompts" });
  await savedPrompts
    .getByText("Write tests for the changes", { exact: true })
    .waitFor();
  await savedPrompts.scrollIntoViewIfNeeded();
  const promptsContrast = await page.evaluate(contrastFailures);
  if (promptsContrast.length)
    throw Error(`Saved prompts contrast: ${JSON.stringify(promptsContrast)}`);
  await page.screenshot({
    path: `${screenshots}/desktop-prompts${suffix}.png`,
  });
  await page.getByRole("button", { name: /^History/ }).click();
  await page
    .getByRole("heading", { name: "Make checkout retries safe" })
    .waitFor();
  const checkContrast = async () => {
    const failures = await page.evaluate(contrastFailures);
    if (failures.length)
      throw Error(`Text contrast: ${JSON.stringify(failures)}`);
  };
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-history${suffix}.png`,
  });
  // Command palette: opens with the keyboard, filters, closes with Escape.
  await page.keyboard.press("ControlOrMeta+k");
  const palette = page.getByRole("dialog", { name: "Search and commands" });
  await palette.waitFor();
  await page.keyboard.type("checkout");
  // Matches inside conversations arrive after a short pause.
  const found = palette.locator(".palette-snippet mark").first();
  await found.waitFor();
  await found.scrollIntoViewIfNeeded();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-palette${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await palette.waitFor({ state: "detached" });
  // Issues and pull requests page.
  await page.getByRole("button", { name: "Issues & PRs" }).click();
  await page.getByRole("list", { name: "Open issues" }).waitFor();
  // Choosing a title shows the issue in full beside the list.
  await page
    .getByRole("list", { name: "Open issues" })
    .getByRole("button", { name: "Retrying a declined card charges twice" })
    .click();
  await page
    .getByRole("region", { name: "Issue #42" })
    .getByText("Seen twice in production", { exact: false })
    .waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-issues${suffix}.png` });
  await resize(960, 640);
  await page.screenshot({
    path: join(tmpdir(), `ais-issues-small${suffix}.png`),
  });
  const issuesLayout = await page.evaluate(() => ({
    body: document.body.scrollWidth > innerWidth,
    page: (() => {
      const el = document.querySelector(".gh-page");
      return el.scrollWidth > el.clientWidth;
    })(),
  }));
  if (issuesLayout.body || issuesLayout.page)
    throw Error(`Issues page overflow: ${JSON.stringify(issuesLayout)}`);
  await resize(1360, 900);
  // New issue: the repository it goes to, a title, a description and labels.
  await page.getByRole("button", { name: /New issue/ }).click();
  const newIssue = page.getByRole("dialog", { name: "New issue" });
  // The title has focus at once, so typing right away is kept.
  await page.keyboard.type("Declined cards are retried");
  await newIssue.getByText("acme/checkout", { exact: true }).waitFor();
  await newIssue
    .getByLabel("Issue description")
    .fill(
      "A declined card is retried three times.\n\n## Done means\n- Declines return at once",
    );
  await newIssue.getByRole("checkbox", { name: "bug" }).check();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-new-issue${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await newIssue.waitFor({ state: "detached" });
  await page.getByRole("button", { name: /Pull requests/ }).click();
  await page.getByRole("list", { name: "Open pull requests" }).waitFor();
  await page
    .getByRole("list", { name: "Open pull requests" })
    .getByRole("button", { name: "Make checkout retries safe" })
    .click();
  await page
    .getByRole("region", { name: "Pull request #44" })
    .getByRole("list", { name: "Checks" })
    .waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-pulls${suffix}.png` });
  // Usage limits page, also at the smallest window.
  await page.getByRole("button", { name: "Usage limits", exact: true }).click();
  await page.getByRole("meter", { name: "Codex: Weekly" }).waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-usage${suffix}.png` });
  await resize(960, 640);
  await page.screenshot({
    path: join(tmpdir(), `ais-usage-small${suffix}.png`),
  });
  const usageOverflow = await page.evaluate(() => {
    const usage = document.querySelector(".usage-page");
    const glance = [...document.querySelectorAll(".agent-limit")];
    return {
      body: document.body.scrollWidth > innerWidth,
      page: usage.scrollWidth > usage.clientWidth,
      glance:
        glance.length !== 2 ||
        glance.some((g) => g.scrollWidth > g.clientWidth),
    };
  });
  if (Object.values(usageOverflow).some(Boolean))
    throw Error(`Usage limits overflow: ${JSON.stringify(usageOverflow)}`);
  // The narrow sidebar keeps every page reachable at the smallest window.
  await page.keyboard.press("ControlOrMeta+b");
  await page.locator(".sidebar.rail").waitFor();
  await checkContrast();
  await page.screenshot({
    path: join(tmpdir(), `ais-rail-small${suffix}.png`),
  });
  const railLayout = await page.evaluate(() => {
    const sidebar = document.querySelector(".sidebar.rail");
    const edge = sidebar.getBoundingClientRect();
    const buttons = [...sidebar.querySelectorAll("button")];
    return {
      width: edge.width,
      body: document.body.scrollWidth > innerWidth,
      clipped: buttons.some((button) => {
        const box = button.getBoundingClientRect();
        return box.left < edge.left || box.right > edge.right;
      }),
      pages: sidebar.querySelectorAll(".sidebar-nav button").length,
      conversations: sidebar.querySelectorAll(".rail-session").length,
    };
  });
  if (
    railLayout.width > 60 ||
    railLayout.body ||
    railLayout.clipped ||
    railLayout.pages !== 7 ||
    railLayout.conversations < 1
  )
    throw Error(`Narrow sidebar layout: ${JSON.stringify(railLayout)}`);
  await page.getByRole("button", { name: "Expand sidebar" }).click();
  await page.locator(".sidebar.rail").waitFor({ state: "detached" });
  await resize(1360, 900);
  // Folded sections say what they hold.
  await page.getByRole("button", { name: "Agents", exact: true }).click();
  await page.getByRole("button", { name: "checkout", exact: true }).click();
  await page.getByRole("button", { name: /^checkout, folded, 1/ }).waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-folded${suffix}.png` });
  await page.getByRole("button", { name: /^Agents, folded/ }).click();
  await page.getByRole("button", { name: /^checkout, folded/ }).click();
  await page.getByRole("button", { name: /^History/ }).click();
  // New task dialog: the task box has focus and fits the window.
  await page.getByRole("button", { name: /New task/ }).click();
  const newTask = page.getByRole("dialog", { name: "New task" });
  await newTask.waitFor();
  await page.keyboard.type("Add retries to the checkout request");
  await newTask.getByRole("button", { name: /Start task/ }).waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-new-task${suffix}.png`,
  });
  // At the smallest window each agent choice still reads on one line.
  await resize(960, 640);
  await page.screenshot({
    path: join(tmpdir(), `ais-new-task-small${suffix}.png`),
  });
  const choices = await page.evaluate(() => {
    const dialog = document
      .getElementById("new-session-title")
      .closest('[role="dialog"]');
    const names = [...dialog.querySelectorAll(".agent-option strong")];
    return {
      sideways: dialog.scrollWidth > dialog.clientWidth,
      count: names.length,
      wrapped: names.some((name) => name.getBoundingClientRect().height > 24),
    };
  });
  if (choices.sideways || choices.wrapped || choices.count !== 3)
    throw Error(`New task agent choices: ${JSON.stringify(choices)}`);
  await resize(1360, 900);
  await page.keyboard.press("Escape");
  await newTask.waitFor({ state: "detached" });
  await resize(960, 640);
  await page.getByRole("button", { name: "Pinned 1" }).click();
  await page
    .getByRole("combobox", { name: "Sort conversations" })
    .selectOption("title");
  await page.screenshot({
    path: join(tmpdir(), `ais-history-small${suffix}.png`),
  });
  const historyOverflow = await page.evaluate(() => ({
    body: document.body.scrollWidth > innerWidth,
    previewWidth: document.querySelector(".conversation").clientWidth,
    rowsHeight: document.querySelector(".session-rows").clientHeight,
  }));
  if (
    historyOverflow.body ||
    historyOverflow.previewWidth < 300 ||
    historyOverflow.rowsHeight < 100
  )
    throw Error(`History layout overflow: ${JSON.stringify(historyOverflow)}`);
  await page.getByRole("button", { name: "All conversations 3" }).click();
  await page
    .getByRole("combobox", { name: "Sort conversations" })
    .selectOption("recent");
  await resize(1360, 900);
  await page
    .getByRole("button", { name: /Make checkout retries safe Codex/ })
    .click();
  await page
    .getByRole("region", { name: "Codex conversation" })
    .getByText("The focused tests pass.", { exact: false })
    .waitFor();
  // The model pill names the model once the catalog is read.
  await page.waitForFunction(() => {
    const pill = document.querySelector(
      '[aria-label="Codex conversation"] button[aria-label^="Model:"]',
    );
    return pill && !pill.getAttribute("aria-label").includes("Loading");
  });
  // The finished turn's activity is folded to a summary. Open it: the latest
  // edit shows its removed and added lines, so they are checked too.
  await page.getByRole("button", { name: /^4 steps · 1 edit/ }).click();
  await page.getByRole("group", { name: "Changed lines" }).waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-chat${suffix}.png` });
  // The message box toolbar, context indicator included, keeps to one row.
  const toolbar = await page.evaluate(() => {
    const footer = document.querySelector(
      '[aria-label="Codex conversation"] .composer-footer',
    );
    return {
      height: footer.getBoundingClientRect().height,
      meter: !!footer.querySelector('[role="meter"]'),
    };
  });
  if (toolbar.height > 48 || !toolbar.meter)
    throw Error(`Message box toolbar: ${JSON.stringify(toolbar)}`);
  // Typing / lists the agent's skills; choosing one only adds it to the text.
  const box = page.getByRole("textbox", { name: "Message agent" }).first();
  await box.click();
  await page.keyboard.type("/re");
  const skillList = page.getByRole("listbox", {
    name: "Saved prompts and skills",
  });
  await skillList.waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-skills${suffix}.png` });
  await page.keyboard.press("Escape");
  await skillList.waitFor({ state: "detached" });
  await box.fill("");
  // A turn can be undone: the files it would change are listed first.
  const chatView = page.getByRole("region", { name: "Codex conversation" });
  await chatView.locator(".chat-messages").evaluate((el) => (el.scrollTop = 0));
  await chatView
    .getByRole("button", { name: "Restore files to before this turn" })
    .click({ force: true });
  const restoreDialog = page.getByRole("dialog", {
    name: "Restore files to before this turn?",
  });
  await restoreDialog.getByText("src/retry.ts").waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-restore${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await restoreDialog.waitFor({ state: "detached" });
  // The run panel shows the project's commands and the latest run.
  const runPanel = page.getByRole("region", { name: "Run a command" });
  await runPanel.getByText("Tests  10 passed").waitFor();
  await runPanel.scrollIntoViewIfNeeded();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-run${suffix}.png` });
  // Comment on a changed line, then see it listed beside the conversation.
  await page
    .getByRole("complementary", { name: "Conversation details" })
    .getByRole("button", { name: "Review all changes" })
    .click();
  const changes = page.getByRole("dialog", { name: "Uncommitted changes" });
  await changes.locator(".diff-line.added .diff-number").first().click();
  await changes
    .getByRole("textbox", { name: /^Comment on src\/checkout\.ts/ })
    .fill("Only retry timeouts and 5xx responses; a declined card is final.");
  await changes.getByRole("button", { name: "Add comment" }).click();
  await changes.locator(".diff-comment").waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-comment${suffix}.png`,
  });
  await changes.getByRole("button", { name: "Close changes" }).click();
  await changes.waitFor({ state: "detached" });
  const comments = page.getByRole("region", { name: "Comments on changes" });
  await comments.scrollIntoViewIfNeeded();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-comments${suffix}.png`,
  });
  await comments.getByRole("button", { name: /^Remove the comment/ }).click();
  await comments.waitFor({ state: "detached" });
  // What changed since a message, read-only.
  await page
    .getByRole("button", { name: "Changes since this message" })
    .first()
    .click();
  const since = page.getByRole("dialog", {
    name: "Changes since this message",
  });
  await since.locator(".diff-line.added").first().waitFor();
  if (await since.locator(".diff-number").count())
    throw Error("The turn diff must be read-only");
  await since.getByRole("button", { name: "Close changes" }).click();
  await since.waitFor({ state: "detached" });
  // A conversation in its own copy of the project.
  await page.evaluate(() => {
    const { source } = window.__aisDemo;
    source.copy = {
      project: "/projects/checkout",
      base: "a1b2c3d4e5f6",
      branch: "ais/7f3c9e21",
      worktree: "/data/copies/7f3c9e21/checkout",
    };
    source.version += 1;
  });
  const copyPanel = page.getByRole("region", { name: "Separate copy" });
  await copyPanel.waitFor();
  await page.getByText("Copy · ais/7f3c9e21").waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-copy${suffix}.png` });
  await copyPanel.getByRole("button", { name: "Bring changes back…" }).click();
  const bring = page.getByRole("dialog", {
    name: "Bring 1 file back into the project?",
  });
  await bring.waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-copy-bring${suffix}.png`,
  });
  await bring.getByRole("button", { name: "Not now" }).click();
  await bring.waitFor({ state: "detached" });
  await page.evaluate(() => {
    const { source } = window.__aisDemo;
    delete source.copy;
    source.version += 1;
  });
  await copyPanel.waitFor({ state: "detached" });
  // Project instructions: each agent's files, editable in the project.
  await page.getByRole("button", { name: /Project instructions/ }).click();
  const instructions = page.getByRole("dialog", {
    name: "Project instructions",
  });
  await instructions.getByLabel("Contents of CLAUDE.md").waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-instructions${suffix}.png`,
  });
  await instructions.getByRole("button", { name: "Close" }).click();
  await instructions.waitFor({ state: "detached" });
  // The model pill opens the model and reasoning choices.
  await page
    .getByRole("region", { name: "Codex conversation" })
    .getByRole("button", { name: /^Model:/ })
    .click();
  const modelPanel = page.getByRole("dialog", { name: "Model and reasoning" });
  await modelPanel.waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-model-panel${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await modelPanel.waitFor({ state: "detached" });
  // Codex's web search is chosen from a globe button's short menu.
  await page.getByRole("button", { name: /^Web search:/ }).click();
  const searchMenu = page.getByRole("menu", { name: "Web search" });
  await searchMenu.waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-web-search${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await searchMenu.waitFor({ state: "detached" });
  // The details panel can be hidden on a wide window too.
  const details = page.getByRole("complementary", {
    name: "Conversation details",
  });
  await page.keyboard.press("ControlOrMeta+Backslash");
  await details.waitFor({ state: "detached" });
  await page.screenshot({
    path: join(tmpdir(), `ais-chat-no-details${suffix}.png`),
  });
  await page.keyboard.press("ControlOrMeta+Backslash");
  await details.waitFor();
  // Attach an image through the file input: it shows as a removable chip.
  await page.getByLabel("Choose images to attach").setInputFiles({
    name: "checkout-error.png",
    mimeType: "image/png",
    buffer: Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
      "base64",
    ),
  });
  await page.getByRole("list", { name: "Attached images" }).waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-attach${suffix}.png`,
  });
  await page.getByRole("button", { name: "Remove checkout-error.png" }).click();
  await page
    .getByRole("list", { name: "Attached images" })
    .waitFor({ state: "detached" });
  // Commit and pull request dialogs say what they do before anything happens.
  await page.getByRole("button", { name: "Commit…" }).click();
  const commit = page.getByRole("dialog", { name: "Commit changes" });
  await commit.getByLabel("Commit message").waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-commit${suffix}.png` });
  await page.keyboard.press("Escape");
  await commit.waitFor({ state: "detached" });
  await page.getByRole("button", { name: "Pull request…" }).click();
  const pullRequest = page.getByRole("dialog", { name: "Open a pull request" });
  await pullRequest.getByLabel("Title").waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-pull-request${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await pullRequest.waitFor({ state: "detached" });
  // Compare view: both sides and one file's two diffs.
  await page.getByRole("button", { name: "Compare", exact: true }).click();
  const compare = page.getByRole("dialog", { name: "Compare" });
  await compare.getByRole("region", { name: "Codex version" }).waitFor();
  await compare.getByRole("button", { name: "src/checkout.ts" }).click();
  await compare
    .getByRole("region", { name: "Codex: src/checkout.ts" })
    .getByText("withRetries", { exact: false })
    .waitFor();
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-compare${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await compare.waitFor({ state: "detached" });
  // Find in conversation: counts matches and marks them in the page.
  await page.keyboard.press("ControlOrMeta+f");
  const find = page.getByRole("textbox", { name: "Find in conversation" });
  await find.waitFor();
  await page.keyboard.type("retr");
  await page
    .getByRole("search")
    .getByText(/^1 of \d+$/)
    .waitFor();
  await page.keyboard.press("Enter");
  await page
    .getByRole("search")
    .getByText(/^2 of \d+$/)
    .waitFor();
  const marked = await page.evaluate(
    () => !("highlights" in CSS) || CSS.highlights.has("find-current"),
  );
  if (!marked) throw Error("The current find match is not highlighted");
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-find${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await find.waitFor({ state: "detached" });
  await page.evaluate(() => {
    const source = window.__aisDemo.source;
    source.approvals = [
      {
        id: "fixture-approval",
        title: "Run command?",
        detail: JSON.stringify({
          command: "npm test -- checkout",
          reason: "Run the checkout tests outside the sandbox",
        }),
        questions: [],
        options: [
          {
            id: "acceptWithExecpolicyAmendment",
            label: "Always allow this command",
            detail:
              "Codex will add a rule to its own settings and stop asking for commands that start with: npm test",
          },
        ],
      },
    ];
    source.status = "waiting";
    source.version++;
  });
  await page.getByRole("button", { name: "Allow once" }).waitFor();
  await checkContrast();
  await resize(960, 640);
  await page.screenshot({ path: join(tmpdir(), `ais-approval${suffix}.png`) });
  const approvalFits = await page.evaluate(
    () =>
      document
        .querySelector(".chat-workspace:not([hidden]) .chat-composer")
        .getBoundingClientRect().bottom <= innerHeight,
  );
  if (!approvalFits) throw Error("Approval composer overflow");
  // The board shows the same request and stays inside the minimum window.
  await page.getByRole("button", { name: /^Agents board/ }).click();
  const board = page.getByRole("region", { name: "Agents board" });
  await board.getByText("Needs approval").waitFor();
  await checkContrast();
  if (await page.evaluate(() => document.body.scrollWidth > innerWidth))
    throw Error("The board overflows the minimum window");
  await page.screenshot({
    path: join(tmpdir(), `ais-board-small${suffix}.png`),
  });
  await resize(1360, 900);
  await page.screenshot({ path: `${screenshots}/desktop-board${suffix}.png` });
  await resize(960, 640);
  await board
    .getByRole("button", { name: "Make checkout retries safe" })
    .click();
  await page.getByRole("button", { name: "Allow once" }).click();
  await page
    .getByRole("button", { name: "Review with Claude Code" })
    .waitFor({ state: "visible" });
  await page.evaluate(() => {
    window.__aisDemo.source.error =
      "Fixture error: retry this response after reconnecting.";
    window.__aisDemo.source.version++;
  });
  await page.getByRole("alert").waitFor();
  await checkContrast();
  await page.evaluate(() => {
    window.__aisDemo.source.error = "";
    window.__aisDemo.source.version++;
  });
  await page.getByRole("alert").waitFor({ state: "hidden" });
  await resize(1360, 900);
  await page.getByRole("button", { name: "Review with Claude Code" }).click();
  await page.getByRole("dialog").waitFor();
  await page.waitForFunction(
    () =>
      document.querySelector('select[aria-label="Reviewer model"]')
        ?.disabled === false,
  );
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-review-preview${suffix}.png`,
  });
  await page
    .getByRole("button", { name: "Send snapshot to Claude Code" })
    .click();
  await page.getByText("Stop retrying permanent payment failures").waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-review${suffix}.png` });
  // Reduced motion turns every transition off; without it they are on.
  const motion = await page.evaluate(() => {
    const dialogRule = [...document.styleSheets]
      .flatMap((sheet) => [...sheet.cssRules])
      .some((rule) => rule.cssText?.includes("motion-rise"));
    const shown = document.querySelector(".chat-workspace:not([hidden])");
    return {
      defined: dialogRule,
      off: getComputedStyle(shown).animationName === "none",
    };
  });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  motion.on = await page.evaluate(
    () =>
      getComputedStyle(document.querySelector(".chat-workspace:not([hidden])"))
        .animationName,
  );
  await page.emulateMedia({ reducedMotion: "reduce" });
  if (!motion.defined || !motion.off || motion.on !== "motion-fade")
    throw Error(`Motion: ${JSON.stringify(motion)}`);
  // Two conversations side by side: the source opens beside its review.
  const sourceItem = page
    .getByRole("navigation", { name: "Open conversations" })
    .locator(".session-item", { hasText: "Make checkout retries safe" });
  await sourceItem.hover();
  await sourceItem
    .getByRole("button", { name: "Open Make checkout retries safe beside" })
    .click();
  await page.locator(".chat-panes.split").waitFor();
  const splitLayout = await page.evaluate(() => ({
    panes: document.querySelectorAll(
      ".chat-panes.split > .chat-workspace:not([hidden])",
    ).length,
    overflow: document.body.scrollWidth > innerWidth,
    // The message box toolbar keeps to one row in each pane.
    toolbars: [
      ...document.querySelectorAll(
        ".chat-workspace:not([hidden]) .composer-footer",
      ),
    ].every((row) => row.getBoundingClientRect().height < 48),
    composers: [
      ...document.querySelectorAll(
        ".chat-workspace:not([hidden]) .chat-composer",
      ),
    ].every(
      (composer) => composer.getBoundingClientRect().bottom <= innerHeight,
    ),
  }));
  if (
    splitLayout.panes !== 2 ||
    splitLayout.overflow ||
    !splitLayout.composers ||
    !splitLayout.toolbars
  )
    throw Error(`Split layout: ${JSON.stringify(splitLayout)}`);
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-split${suffix}.png` });
  // A narrow window shows only the focused pane.
  await resize(960, 640);
  if ((await page.locator(".chat-workspace:not([hidden])").count()) !== 1)
    throw Error("A narrow window must show one pane");
  await resize(1360, 900);
  await page
    .locator(".chat-workspace.focused")
    .getByRole("button", { name: "Close this pane" })
    .click();
  await page.locator(".chat-panes.split").waitFor({ state: "detached" });
  await page.getByText("Stop retrying permanent payment failures").waitFor();
  await page.getByRole("button", { name: "src/checkout.ts:6" }).click();
  if ((await page.locator(".diff-line.focused").count()) !== 1)
    throw Error("Missing finding highlight");
  await resize(960, 640);
  await page.screenshot({
    path: join(tmpdir(), `ais-review-small${suffix}.png`),
  });
  await page
    .getByRole("button", { name: "Close Review with Claude Code", exact: true })
    .click();
  await page.getByRole("button", { name: "Saved workspace" }).click();
  await page.getByLabel("Saved work type").selectOption("reviews");
  await page
    .getByRole("button", { name: "Reopen Review with Claude Code" })
    .waitFor();
  const libraryOverflow = await page.evaluate(() => {
    const dialog = document.querySelector("dialog");
    const bounds = dialog.getBoundingClientRect();
    return (
      bounds.bottom > innerHeight ||
      bounds.top < 0 ||
      dialog.scrollWidth > dialog.clientWidth
    );
  });
  if (libraryOverflow)
    throw Error("Saved workspace overflows the minimum window size");
  await page.screenshot({
    path: join(tmpdir(), `ais-saved-workspace-small${suffix}.png`),
  });
  await resize(1360, 900);
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-saved-workspace${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", { name: "Saved workspace" })
    .waitFor({ state: "visible" });
  if (await page.getByRole("dialog").count())
    throw Error("Escape did not close the saved workspace");
  await page.getByRole("button", { name: "Saved workspace" }).click();
  await page.getByLabel("Saved work type").selectOption("reviews");
  await page
    .getByRole("button", { name: "Reopen Review with Claude Code" })
    .click();
  await page.getByText("Stop retrying permanent payment failures").waitFor();
  await resize(960, 640);
  const overflow = await page.evaluate(() => ({
    body: document.body.scrollWidth > innerWidth,
    composer:
      document
        .querySelector(".chat-workspace:not([hidden]) .chat-composer")
        .getBoundingClientRect().bottom > innerHeight,
    messagesHeight: document.querySelector(
      ".chat-workspace:not([hidden]) .chat-messages",
    ).clientHeight,
  }));
  console.log(JSON.stringify({ engine, errors, overflow, historyOverflow }));

  if (errors.length || overflow.body || overflow.composer) process.exitCode = 1;
} finally {
  await browser.close();
}
