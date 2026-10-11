// Run against the local Vite preview. All provider responses below are fictional.
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { tmpdir } from "node:os";
const { chromium } = await import(
  process.env.AIS_PLAYWRIGHT_MODULE || "playwright"
);
const browser = await chromium.launch({ channel: "chrome", headless: true });
try {
  const page = await browser.newPage({
    viewport: { width: 1360, height: 900 },
    deviceScaleFactor: 1,
  });
  const errors = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  await page.addInitScript(() => {
    const project = "/projects/checkout";
    const source = {
      id: "chat",
      tool: "codex",
      project,
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
        },
        {
          id: "a",
          role: "assistant",
          kind: "text",
          model: "codex-demo",
          text: "I added bounded retries and an idempotency key for each checkout attempt. The request now uses the same key across retries, so a timeout will not create a second charge.",
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
          text: "The focused tests pass. The changes are ready for a second review.\n\n- Reuse one request key across retries\n- Retry transient failures up to three times\n- Return the last response to the caller",
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
    };
    window.go = {
      main: {
        App: {
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
            sessions: [session],
            projects: [
              {
                path: project,
                name: "checkout",
                sessions: 1,
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
          ReadChat: async (id, version) => ({
            version: chats[id].version,
            state: version === chats[id].version ? null : chats[id],
          }),
          ResumeChat: async () => source,
          NewChat: async () => source,
          PickProject: async () => "",
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
          CloseChat: async () => {},
          AnswerChat: async () => {},
        },
      },
    };
  });
  await page.goto(process.argv[2] || "http://127.0.0.1:5183");
  await page.getByRole("button", { name: /Codex · checkout/ }).click();
  await page
    .getByRole("region", { name: "Codex conversation" })
    .getByText("The focused tests pass.", { exact: false })
    .waitFor();
  const screenshots = fileURLToPath(
    new URL("../docs/screenshots/", import.meta.url),
  );
  await page.waitForFunction(
    () =>
      document.querySelector('select[aria-label="Model"]')?.disabled === false,
  );
  await page.screenshot({ path: `${screenshots}/desktop-chat.png` });
  await page.getByRole("button", { name: "Review with Claude Code" }).click();
  await page.getByRole("dialog").waitFor();
  await page.waitForFunction(
    () =>
      document.querySelector('select[aria-label="Reviewer model"]')
        ?.disabled === false,
  );
  await page.screenshot({ path: `${screenshots}/desktop-review-preview.png` });
  await page
    .getByRole("button", { name: "Send snapshot to Claude Code" })
    .click();
  await page.getByText("Stop retrying permanent payment failures").waitFor();
  await page.screenshot({ path: `${screenshots}/desktop-review.png` });
  await page.getByRole("button", { name: "src/checkout.ts:6" }).click();
  if ((await page.locator(".diff-line.focused").count()) !== 1)
    throw Error("Missing finding highlight");
  await page.setViewportSize({ width: 960, height: 640 });
  await page.screenshot({ path: join(tmpdir(), "ais-review-small.png") });
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
  console.log(JSON.stringify({ errors, overflow }));

  if (errors.length || overflow.body || overflow.composer) process.exitCode = 1;
} finally {
  await browser.close();
}
