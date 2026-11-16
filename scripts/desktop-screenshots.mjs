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
                updatedAt: "2026-10-04T09:00:00Z",
              },
              {
                number: 39,
                title: "Document the retry contract for API clients",
                url: "https://github.com/acme/checkout/issues/39",
                author: "bo",
                labels: ["documentation"],
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
          Transcript: async () => ({
            messages: source.messages,
            truncated: false,
          }),
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
          ChatDiff: async () => snapshot,
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
  await checkContrast();
  await page.screenshot({
    path: `${screenshots}/desktop-palette${suffix}.png`,
  });
  await page.keyboard.press("Escape");
  await palette.waitFor({ state: "detached" });
  // Issues and pull requests page.
  await page.getByRole("button", { name: "Issues & PRs" }).click();
  await page.getByRole("list", { name: "Open issues" }).waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-issues${suffix}.png` });
  await page.getByRole("button", { name: /Pull requests/ }).click();
  await page.getByRole("list", { name: "Open pull requests" }).waitFor();
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
  await page.waitForFunction(
    () =>
      document.querySelector('select[aria-label="Model"]')?.disabled === false,
  );
  // The finished turn's activity is folded to a summary. Open it: the latest
  // edit shows its removed and added lines, so they are checked too.
  await page.getByRole("button", { name: /^4 steps · 1 edit/ }).click();
  await page.getByRole("group", { name: "Changed lines" }).waitFor();
  await checkContrast();
  await page.screenshot({ path: `${screenshots}/desktop-chat${suffix}.png` });
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
