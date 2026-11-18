import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, ChatState, Run, RunTaskStatus, Tool } from "../api";
import Board from "./Board";
import { runTaskState } from "./runTasks";

const chat = (id: string, patch: Partial<ChatState>): ChatState =>
  ({
    id,
    tool: "codex",
    project: "/work/checkout",
    title: `Task ${id}`,
    status: "idle",
    error: "",
    messages: [],
    approvals: [],
    ...patch,
  }) as ChatState;
const tools: Tool[] = [
  { name: "codex", path: "/bin/codex", available: true },
  { name: "claude", path: "", available: false },
];
const answer = vi.fn();
type FakeRun = { run: Run; tasks: RunTaskStatus[]; fail?: boolean };
let runs: FakeRun[] = [];
const run = (id: string, createdAt: string): Run => ({
  id,
  tool: "claude",
  repository: "/work/checkout",
  base: "abc",
  createdAt,
  notifications: true,
  tasks: [],
});
const task = (
  window: string,
  patch: Partial<RunTaskStatus>,
): RunTaskStatus => ({
  task: { window, prompt: `Prompt ${window}`, branch: "b", path: "/p" },
  state: "running",
  attention: false,
  quiet: false,
  files: [],
  ...patch,
});
beforeEach(() => {
  answer.mockReset().mockResolvedValue(undefined);
  runs = [];
  window.go = {
    main: {
      App: {
        AnswerChat: answer,
        ListRuns: async () => runs.map((item) => item.run),
        RunStatus: async (id: string) => {
          const item = runs.find((entry) => entry.run.id === id)!;
          if (item.fail) throw new Error("tmux is not running");
          return { id, base: "abc", tasks: item.tasks };
        },
      } as unknown as Backend,
    },
  };
});

test("groups conversations by what they need, with Needs you first", () => {
  render(
    <Board
      tools={tools}
      onOpen={vi.fn()}
      onOpenRun={vi.fn()}
      chats={[
        chat("a", { status: "running", activeModel: "codex-demo" }),
        chat("b", { status: "error", error: "The agent exited (1)." }),
        chat("c", { tool: "claude", status: "stopped" }),
      ]}
    />,
  );
  const headings = screen
    .getAllByRole("heading", { level: 2 })
    .map((h) => h.textContent);
  expect(headings).toEqual(["Needs you0", "Working1", "Ready2"]);
  // Empty columns stay in place so the layout does not jump.
  expect(screen.getByText("Nothing is waiting for you.")).toBeVisible();
  expect(screen.getByText("The agent exited (1).")).toBeVisible();
  expect(screen.getByText(/Stopped by you/)).toBeVisible();
  expect(screen.getByText("1 working · 0 waiting · 2 open")).toBeVisible();
  expect(screen.getByText("Not found")).toBeVisible();
});

test("approves or denies a simple request in place and opens the conversation on request", async () => {
  const open = vi.fn();
  render(
    <Board
      tools={tools}
      onOpen={open}
      onOpenRun={vi.fn()}
      chats={[
        chat("a", {
          status: "waiting",
          approvals: [
            {
              id: "req-1",
              title: "Run checkout tests",
              detail: "npm test -- checkout",
              questions: [],
            },
          ],
        }),
      ]}
    />,
  );
  const card = screen.getByRole("article", { name: "Task a, Codex" });
  expect(within(card).getByText("Needs approval")).toBeVisible();
  expect(within(card).getByText("npm test -- checkout")).toBeVisible();
  await userEvent.click(within(card).getByRole("button", { name: "Deny" }));
  expect(answer).toHaveBeenCalledWith("a", "req-1", false, {});
  await userEvent.click(within(card).getByRole("button", { name: "Task a" }));
  expect(open).toHaveBeenCalledWith("a");
});

test("sends questions to the conversation and reports a failed answer", async () => {
  const open = vi.fn();
  answer.mockRejectedValue(new Error("This request was already answered."));
  render(
    <Board
      tools={tools}
      onOpen={open}
      onOpenRun={vi.fn()}
      chats={[
        chat("q", {
          status: "waiting",
          approvals: [
            {
              id: "req-q",
              title: "Choose a target",
              detail: "",
              questions: [{ id: "t", text: "Which target?", options: [] }],
            },
          ],
        }),
        chat("a", {
          status: "waiting",
          approvals: [
            {
              id: "req-a",
              title: "Delete dist",
              detail: "rm -rf dist",
              questions: [],
            },
          ],
        }),
      ]}
    />,
  );
  const question = screen.getByRole("article", { name: "Task q, Codex" });
  expect(within(question).getByText("Has a question")).toBeVisible();
  expect(
    within(question).queryByRole("button", { name: "Allow once" }),
  ).not.toBeInTheDocument();
  await userEvent.click(
    within(question).getByRole("button", { name: "Reply in conversation" }),
  );
  expect(open).toHaveBeenCalledWith("q");
  const approval = screen.getByRole("article", { name: "Task a, Codex" });
  await userEvent.click(
    within(approval).getByRole("button", { name: "Allow once" }),
  );
  expect(await within(approval).findByRole("alert")).toHaveTextContent(
    "already answered",
  );
});

test("shows parallel-run tasks in the same columns, with process state and attention kept apart", async () => {
  runs = [
    {
      run: run("ais-new", "2026-11-15T10:00:00Z"),
      tasks: [
        task("task-1", {
          attention: true,
          files: [{ status: "M", path: "a.ts" }],
        }),
        task("task-2", {}),
        task("task-3", {
          state: "exited",
          exitCode: 0,
          files: [
            { status: "M", path: "a.ts" },
            { status: "A", path: "b.ts" },
          ],
        }),
        task("task-4", { state: "missing", error: "The worktree is missing." }),
      ],
    },
    { run: run("ais-broken", "2026-11-14T10:00:00Z"), tasks: [], fail: true },
  ];
  const openRun = vi.fn();
  render(
    <Board tools={tools} chats={[]} onOpen={vi.fn()} onOpenRun={openRun} />,
  );
  const needs = await screen.findByRole("article", {
    name: "Prompt task-1, parallel run task",
  });
  // Still running, and separately marked as wanting attention.
  expect(within(needs).getByText("Running")).toBeVisible();
  expect(within(needs).getByText("Agent alert")).toBeVisible();
  expect(within(needs).getByText("1 changed file to review")).toBeVisible();
  const headings = screen
    .getAllByRole("heading", { level: 2 })
    .map((h) => h.textContent);
  expect(headings).toEqual(["Needs you1", "Working1", "Ready2"]);
  const done = screen.getByRole("article", {
    name: "Prompt task-3, parallel run task",
  });
  expect(within(done).getByText("Finished")).toBeVisible();
  expect(runTaskState({ state: "exited", exitCode: 2 } as RunTaskStatus)).toBe(
    "Ended with an error (2)",
  );
  expect(within(done).getByText("2 changed files to review")).toBeVisible();
  expect(screen.getByText("The worktree is missing.")).toBeVisible();
  // A run that cannot be inspected is reported without hiding the others.
  expect(screen.getByRole("status")).toHaveTextContent(
    "ais-broken: tmux is not running",
  );
  await userEvent.click(
    within(done).getByRole("button", { name: "Prompt task-3" }),
  );
  expect(openRun).toHaveBeenCalledWith("ais-new", "task-3");
});
