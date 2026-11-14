// Local fictional runs only: no tmux, agents, or provider accounts are contacted.
import { mkdir } from "node:fs/promises";
const { chromium } = await import(
  process.env.AIS_PLAYWRIGHT_MODULE || "playwright"
);
const browser = await chromium.launch({ channel: "chrome", headless: true });
const output = process.env.AIS_SCREENSHOT_DIR || "/tmp/ais-run-check";
await mkdir(output, { recursive: true });
const mode = process.env.AIS_APPEARANCE || "light";
try {
  const page = await browser.newPage({
    viewport: { width: 1360, height: 900 },
  });
  const errors = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.addInitScript((mode) => {
    localStorage.setItem("ais:appearance:v1", mode);
    const tasks = [
      {
        window: "task-1",
        path: "/work/checkout/task-1",
        branch: "ais-demo/task-1",
        prompt: "Bound checkout retries and preserve the request key",
      },
      {
        window: "task-2",
        path: "/work/checkout/task-2",
        branch: "ais-demo/task-2",
        prompt: "Test permanent payment failures",
      },
      {
        window: "task-3",
        path: "/work/checkout/task-3",
        branch: "ais-demo/task-3",
        prompt: "Document the retry contract",
      },
    ];
    const run = {
      id: "ais-demo",
      tool: "codex",
      repository: "/projects/checkout",
      base: "a1b2c3d4e5f6",
      createdAt: "2026-11-14T08:30:00Z",
      notifications: true,
      tasks,
    };
    const state = {
      runs: [run],
      launchCalls: [],
      stopCalls: [],
      failList: false,
      tasks: tasks.map((task, i) => ({
        task,
        state: i === 0 ? "running" : "exited",
        exitCode: i === 0 ? undefined : 0,
        attention: i === 0,
        quiet: false,
        currentBranch: task.branch,
        pane: "%1",
        files:
          i === 2
            ? []
            : [
                {
                  status: "M",
                  path: i === 0 ? "src/checkout.ts" : "test/checkout.test.ts",
                },
                { status: "?", path: "notes.txt" },
              ],
        error: i === 2 ? "Fixture worktree is temporarily unavailable" : "",
      })),
    };
    window.__aisRuns = state;
    window.go = {
      main: {
        App: {
          SetAppearance: async () => {},
          Refresh: async () => ({
            sessions: [],
            projects: [
              {
                name: "checkout",
                path: run.repository,
                available: true,
                recent: true,
                sessions: 0,
              },
            ],
            tools: [
              { name: "codex", path: "/fixture/codex", available: true },
              { name: "claude", path: "/fixture/claude", available: true },
            ],
            warnings: [],
          }),
          ListChats: async () => [],
          Search: async () => [],
          ListRuns: async () => {
            if (state.failList) throw Error("Fixture run storage unavailable");
            return structuredClone(state.runs);
          },
          RunStatus: async (id) => ({
            id,
            base: run.base,
            tasks: structuredClone(state.tasks),
          }),
          RunDiff: async (_id, task, file) => ({
            patch: `diff --git a/${file || "src/checkout.ts"} b/${file || "src/checkout.ts"}\n@@ -1,3 +1,5 @@\n export function checkout(order) {\n-  return charge(order);\n+  const key = crypto.randomUUID();\n+  return retry(() => charge(order, { key }));\n }\n`,
            truncated: false,
          }),
          StopRun: async (id) => {
            state.stopCalls.push(id);
            state.tasks.forEach((task) => {
              task.state = "missing";
              task.attention = false;
            });
          },
          StartRun: async (project, tool, prompts) => {
            state.launchCalls.push({ project, tool, prompts });
            await new Promise((resolve) => setTimeout(resolve, 100));
            const next = {
              ...run,
              id: "ais-new-fixture",
              repository: project,
              tool,
              tasks: prompts.map((prompt, i) => ({ ...tasks[i], prompt })),
            };
            state.runs.unshift(next);
            return structuredClone(next);
          },
          PickProject: async () => "",
        },
      },
      RunAttachCommand: async (id, task) => `tmux attach-session -t %1`,
    };
  }, mode);
  await page.goto(process.argv[2] || "http://127.0.0.1:5183");
  await page
    .getByRole("button", { name: "Parallel runs", exact: true })
    .click();
  await page
    .getByText("+  const key = crypto.randomUUID();", { exact: true })
    .waitFor();
  await page.getByRole("button", { name: /task-2.*Test permanent/ }).click();
  await page.getByRole("button", { name: /test\/checkout.test.ts/ }).click();
  await page.getByRole("button", { name: "Refresh runs" }).click();
  await page.getByRole("button", { name: /test\/checkout.test.ts/ }).waitFor();
  if (
    (await page
      .getByRole("button", { name: /task-2.*Test permanent/ })
      .getAttribute("aria-pressed")) !== "true"
  )
    throw Error("Refresh lost task selection");
  if (
    !(await page
      .getByRole("button", { name: /test\/checkout.test.ts/ })
      .evaluate((button) => button.classList.contains("selected")))
  )
    throw Error("Refresh lost file selection");
  await page.getByRole("button", { name: /task-1.*Bound checkout/ }).click();
  await page.getByRole("button", { name: /src\/checkout.ts/ }).click();
  await page.screenshot({ path: `${output}/desktop-runs-${mode}.png` });
  await page.setViewportSize({ width: 960, height: 640 });
  await page.reload();
  await page
    .getByRole("button", { name: "Parallel runs", exact: true })
    .click();
  await page.getByRole("button", { name: /src\/checkout.ts/ }).click();
  await page
    .getByText("+  const key = crypto.randomUUID();", { exact: true })
    .waitFor();
  await page.screenshot({ path: `${output}/desktop-runs-small-${mode}.png` });
  const layout = await page.evaluate(() => ({
    overflow: document.body.scrollWidth > innerWidth,
    codeHeight: document.querySelector(".runs-workspace .diff-code")
      .clientHeight,
    bottom:
      document.querySelector(".runs-workspace").getBoundingClientRect().bottom >
      innerHeight,
  }));
  if (layout.overflow || layout.bottom || layout.codeHeight < 70)
    throw Error(`Run layout: ${JSON.stringify(layout)}`);
  await page.getByRole("button", { name: /task-3.*Document/ }).click();
  await page
    .getByRole("alert")
    .filter({ hasText: "Fixture worktree" })
    .waitFor();
  await page.getByRole("button", { name: /task-1.*Bound checkout/ }).click();
  await page.getByRole("button", { name: "Stop run…" }).click();
  await page.getByRole("button", { name: "Keep running" }).click();
  if (await page.evaluate(() => window.__aisRuns.stopCalls.length))
    throw Error("Stop confirmation was bypassed");
  await page.getByRole("button", { name: "Stop run…" }).click();
  await page.getByRole("button", { name: "Stop agents" }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page.getByRole("button", { name: /src\/checkout.ts/ }).waitFor();
  await page.getByRole("button", { name: "New parallel run" }).click();
  await page
    .getByLabel("Task 1", { exact: true })
    .fill("Fix checkout\nKeep the API");
  await page.getByLabel("Task 2", { exact: true }).fill("Add regression tests");
  await page.screenshot({ path: `${output}/desktop-run-launch-${mode}.png` });
  await page
    .getByRole("button", { name: "Launch 2 agents" })
    .evaluate((button) => {
      button.click();
      button.click();
    });
  await page
    .getByRole("heading", { name: "ais-new-fixture", exact: true })
    .waitFor();
  const calls = await page.evaluate(() => window.__aisRuns.launchCalls);
  if (
    calls.length !== 1 ||
    calls[0].prompts[0] !== "Fix checkout\nKeep the API"
  )
    throw Error("Launch prompts or duplicate guard failed");
  if (errors.length) throw Error(JSON.stringify(errors));
  console.log(
    JSON.stringify({ mode, layout, launchCalls: calls.length, errors }),
  );
} finally {
  await browser.close();
}
