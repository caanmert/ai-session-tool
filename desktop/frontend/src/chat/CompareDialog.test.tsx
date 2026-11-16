import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import ChatWorkspace from "../ChatWorkspace";
import type { Backend, ChatState, ComparisonView } from "../api";
import CompareDialog from "./CompareDialog";

const view: ComparisonView = {
  id: "cmp-1",
  project: "/work/app",
  base: "a1b2c3d4e5f6",
  prompt: "Add a greeting\nwith tests",
  sides: [
    {
      tool: "claude",
      chatId: "chat-claude",
      worktree: "/data/worktrees/cmp-1/app-claude",
      branch: "ais-compare/1/claude",
      status: "idle",
      changes: {
        available: true,
        files: [
          { path: "a.txt", status: "M", added: 1, removed: 1 },
          { path: "hello.txt", status: "?", added: 1, removed: 0 },
        ],
        added: 2,
        removed: 1,
      },
    },
    {
      tool: "codex",
      chatId: "chat-codex",
      worktree: "/data/worktrees/cmp-1/app-codex",
      branch: "ais-compare/1/codex",
      status: "running",
      changes: {
        available: true,
        files: [{ path: "hello.txt", status: "?", added: 2, removed: 0 }],
        added: 2,
        removed: 0,
      },
    },
  ],
};
const snapshot = (path: string, line: string) => ({
  root: "/w",
  base: "a1b2c3d",
  hash: "h",
  diff: "",
  warnings: [],
  files: [{ path, patch: `@@ -0,0 +1 @@\n+${line}\n` }],
});
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    ComparisonOverview: vi.fn().mockResolvedValue(view),
    ComparisonDiff: vi.fn(async (_id: string, tool: string) =>
      snapshot("hello.txt", tool === "claude" ? "hello" : "hi"),
    ),
    UseComparisonSide: vi.fn().mockResolvedValue(["a.txt", "hello.txt"]),
    RemoveComparison: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});
const show = (props: Partial<Parameters<typeof CompareDialog>[0]> = {}) =>
  render(
    <CompareDialog
      id="cmp-1"
      onClose={vi.fn()}
      onOpenChat={vi.fn()}
      onRemoved={vi.fn()}
      {...props}
    />,
  );

test("shows both sides and each file's changes side by side", async () => {
  const openChat = vi.fn();
  show({ onOpenChat: openChat });
  const claude = await screen.findByRole("region", {
    name: "Claude Code version",
  });
  expect(within(claude).getByText("Ready")).toBeVisible();
  expect(within(claude).getByText("ais-compare/1/claude")).toBeVisible();
  const codex = screen.getByRole("region", { name: "Codex version" });
  expect(within(codex).getByText("Working")).toBeVisible();
  // A side that is still working cannot be used yet.
  expect(
    within(codex).getByRole("button", { name: "Use this version…" }),
  ).toBeDisabled();
  const table = screen.getByRole("table");
  const aRow = within(table)
    .getByRole("button", { name: "a.txt" })
    .closest("tr")!;
  expect(within(aRow).getByText("No change")).toBeVisible();
  await userEvent.click(
    within(table).getByRole("button", { name: "hello.txt" }),
  );
  expect(
    await screen.findByRole("region", { name: "Claude Code: hello.txt" }),
  ).toHaveTextContent("hello");
  expect(
    screen.getByRole("region", { name: "Codex: hello.txt" }),
  ).toHaveTextContent("hi");
  await userEvent.click(
    within(claude).getByRole("button", { name: "Open conversation" }),
  );
  expect(openChat).toHaveBeenCalledWith("chat-claude");
});

test("copies a side into the project only after confirming", async () => {
  show();
  const claude = await screen.findByRole("region", {
    name: "Claude Code version",
  });
  await userEvent.click(
    within(claude).getByRole("button", { name: "Use this version…" }),
  );
  const confirm = await screen.findByRole("dialog", {
    name: "Use Claude Code's version?",
  });
  expect(within(confirm).getByText(/Nothing is committed/)).toBeVisible();
  expect(within(confirm).getByRole("button", { name: "Cancel" })).toHaveFocus();
  expect(backend.UseComparisonSide).not.toHaveBeenCalled();
  await userEvent.click(
    within(confirm).getByRole("button", { name: "Copy into project" }),
  );
  expect(backend.UseComparisonSide).toHaveBeenCalledWith("cmp-1", "claude");
  expect(
    await screen.findByText(/Copied 2 files from Claude Code/),
  ).toBeVisible();
});

test("removing asks first and says what is lost", async () => {
  backend.ComparisonOverview.mockResolvedValue({
    ...view,
    sides: view.sides.map((side) => ({ ...side, status: "idle" })),
  });
  const removed = vi.fn();
  const close = vi.fn();
  show({ onRemoved: removed, onClose: close });
  await screen.findByRole("region", { name: "Codex version" });
  await userEvent.click(
    screen.getByRole("button", { name: "Remove comparison…" }),
  );
  const confirm = await screen.findByRole("dialog", {
    name: "Remove this comparison?",
  });
  expect(within(confirm).getByText(/are lost/)).toBeVisible();
  expect(
    within(confirm).getByRole("button", { name: "Keep it" }),
  ).toHaveFocus();
  await userEvent.click(
    within(confirm).getByRole("button", { name: "Remove comparison" }),
  );
  await waitFor(() =>
    expect(removed).toHaveBeenCalledWith(["chat-claude", "chat-codex"]),
  );
  expect(close).toHaveBeenCalled();
});

test("a refused copy is explained and nothing else happens", async () => {
  backend.UseComparisonSide.mockRejectedValue(
    new Error("your project has its own uncommitted changes in a.txt"),
  );
  show();
  const claude = await screen.findByRole("region", {
    name: "Claude Code version",
  });
  await userEvent.click(
    within(claude).getByRole("button", { name: "Use this version…" }),
  );
  await userEvent.click(
    within(
      await screen.findByRole("dialog", { name: "Use Claude Code's version?" }),
    ).getByRole("button", { name: "Copy into project" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "own uncommitted changes in a.txt",
  );
});

test("a conversation that is part of a comparison opens the compare view", async () => {
  const chat = {
    id: "chat-claude",
    tool: "claude",
    project: "/data/worktrees/cmp-1/app-claude",
    title: "Add a greeting",
    status: "idle",
    error: "",
    comparison: "cmp-1",
    messages: [],
    approvals: [],
    shortened: false,
    version: 1,
  } as unknown as ChatState;
  Object.assign(backend, {
    ReadChat: vi.fn(async (_id: string, version: number) => ({
      version: 1,
      state: version === 1 ? null : chat,
    })),
    ChatModels: vi
      .fn()
      .mockResolvedValue({ models: [], defaultModel: "", defaultEffort: "" }),
    ChatChanges: vi
      .fn()
      .mockResolvedValue({ available: true, files: [], added: 0, removed: 0 }),
  });
  const select = vi.fn();
  render(
    <ChatWorkspace
      chat={chat}
      active
      reviewerAvailable
      onOpen={vi.fn()}
      onSelect={select}
      onState={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Compare" }));
  const dialog = await screen.findByRole("dialog", { name: "Compare" });
  expect(backend.ComparisonOverview).toHaveBeenCalledWith("cmp-1");
  await userEvent.click(
    within(
      await within(dialog).findByRole("region", { name: "Codex version" }),
    ).getByRole("button", { name: "Open conversation" }),
  );
  expect(select).toHaveBeenCalledWith("chat-codex");
});
