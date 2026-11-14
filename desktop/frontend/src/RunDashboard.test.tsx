import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RunDashboard from "./RunDashboard";
import type { Backend, Run, RunStatus, RunPatch } from "./api";

const run: Run = {
  id: "ais-one",
  tool: "codex",
  repository: "/projects/api",
  base: "abcdef1234567890",
  createdAt: "2026-11-14T00:00:00Z",
  notifications: true,
  tasks: [
    {
      window: "task-1",
      path: "/work/task-1",
      branch: "task-1",
      prompt: "Fix retries",
    },
    {
      window: "task-2",
      path: "/work/task-2",
      branch: "task-2",
      prompt: "Add tests",
    },
  ],
};
const status: RunStatus = {
  id: run.id,
  base: run.base,
  tasks: run.tasks.map((task, i) => ({
    task,
    state: i === 0 ? "running" : "exited",
    exitCode: i === 1 ? 0 : undefined,
    attention: i === 0,
    quiet: false,
    currentBranch: task.branch,
    pane: "%1",
    files: [
      { status: "M", path: "app.txt" },
      { status: "?", path: "odd [1].txt" },
    ],
  })),
};
const props = {
  project: "",
  projects: [
    {
      path: run.repository,
      name: "api",
      sessions: 1,
      available: true,
      recent: false,
    },
  ],
  tools: [{ name: "codex" as const, available: true, path: "/fixture/codex" }],
};
let backend: Backend;
beforeEach(() => {
  backend = {
    ListRuns: vi.fn().mockResolvedValue([run]),
    RunStatus: vi.fn().mockResolvedValue(status),
    RunDiff: vi.fn().mockResolvedValue({
      patch: "@@ -1 +1 @@\n-before\n+after",
      truncated: false,
    }),
    StartRun: vi.fn().mockResolvedValue(run),
    StopRun: vi.fn().mockResolvedValue(undefined),
    RunAttachCommand: vi.fn().mockResolvedValue("tmux attach-session -t %1"),
    PickProject: vi.fn(),
  } as unknown as Backend;
  window.go = { main: { App: backend } };
});
describe("parallel run dashboard", () => {
  it("copies a task-specific attach command without launching another agent", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    render(<RunDashboard {...props} />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Copy attach command" }),
    );
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith("tmux attach-session -t %1"),
    );
    expect(backend.RunAttachCommand).toHaveBeenCalledWith(run.id, "task-1");
    expect(backend.StartRun).not.toHaveBeenCalled();
  });
  it("shows task state and alerts separately, and preserves task/file selection on refresh", async () => {
    render(<RunDashboard {...props} />);
    await screen.findByText("Agent event");
    const tasks = screen.getByLabelText("Run tasks");
    await userEvent.click(
      within(tasks).getByRole("button", { name: /task-2/ }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: /odd \[1\].txt/ }),
    );
    await waitFor(() =>
      expect(backend.RunDiff).toHaveBeenLastCalledWith(
        run.id,
        "task-2",
        "odd [1].txt",
      ),
    );
    await userEvent.click(screen.getByRole("button", { name: "Refresh runs" }));
    await waitFor(() => expect(backend.RunStatus).toHaveBeenCalledTimes(2));
    expect(
      within(tasks).getByRole("button", { name: /task-2/ }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /odd \[1\].txt/ })).toHaveClass(
      "selected",
    );
    expect(backend.StartRun).not.toHaveBeenCalled();
    expect(backend.StopRun).not.toHaveBeenCalled();
  });
  it("does not show a stale patch under a newly selected file", async () => {
    let finish!: (patch: RunPatch) => void;
    backend.RunDiff = vi.fn((_id, _task, file) =>
      file === "app.txt"
        ? new Promise<RunPatch>((resolve) => {
            finish = resolve;
          })
        : Promise.resolve({ patch: "+Newest patch", truncated: false }),
    );
    render(<RunDashboard {...props} />);
    await screen.findByText("Agent event");
    await userEvent.click(screen.getByRole("button", { name: "M app.txt" }));
    expect(screen.queryByText("+Newest patch")).not.toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: /odd \[1\].txt/ }),
    );
    await screen.findByText("+Newest patch");
    await act(async () => finish({ patch: "+Stale patch", truncated: false }));
    expect(screen.queryByText("+Stale patch")).not.toBeInTheDocument();
  });
  it("requires explicit stop confirmation and keeps failures visible", async () => {
    backend.StopRun = vi.fn().mockRejectedValue(new Error("tmux unavailable"));
    render(<RunDashboard {...props} />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Stop run…" }),
    );
    expect(backend.StopRun).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Keep running" }));
    expect(backend.StopRun).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Stop run…" }));
    await userEvent.click(screen.getByRole("button", { name: "Stop agents" }));
    expect(backend.StopRun).toHaveBeenCalledWith(run.id);
    expect(await screen.findByRole("dialog")).toHaveTextContent(
      "tmux unavailable",
    );
  });
  it("launches multiline task prompts once and keeps setup errors recoverable", async () => {
    let finish!: () => void;
    backend.StartRun = vi.fn(
      () =>
        new Promise<Run>((_resolve, reject) => {
          finish = () => reject(new Error("saved work is preserved"));
        }),
    );
    render(<RunDashboard {...props} />);
    await userEvent.click(
      screen.getByRole("button", { name: "New parallel run" }),
    );
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Task 1"), {
      target: { value: "Fix retries\nKeep the API" },
    });
    fireEvent.change(within(dialog).getByLabelText("Task 2"), {
      target: { value: "Add tests" },
    });
    const launch = within(dialog).getByRole("button", {
      name: "Launch 2 agents",
    });
    fireEvent.click(launch);
    fireEvent.click(launch);
    expect(backend.StartRun).toHaveBeenCalledTimes(1);
    expect(backend.StartRun).toHaveBeenCalledWith(run.repository, "codex", [
      "Fix retries\nKeep the API",
      "Add tests",
    ]);
    const cancel = new Event("cancel", { cancelable: true });
    fireEvent(dialog, cancel);
    expect(cancel.defaultPrevented).toBe(true);
    await act(async () => finish());
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "saved work is preserved",
    );
    expect(within(dialog).getByLabelText("Task 1")).toHaveValue(
      "Fix retries\nKeep the API",
    );
  });
  it("still reviews files when tmux is unavailable and explains missing worktrees", async () => {
    backend.RunStatus = vi.fn().mockResolvedValue({
      ...status,
      warning: "tmux absent",
      tasks: [
        { ...status.tasks[0], state: "unknown" },
        { ...status.tasks[1], error: "worktree missing", files: [] },
      ],
    });
    render(<RunDashboard {...props} />);
    await screen.findByText(/Pane inspection: tmux absent/);
    await screen.findByText("+after");
    await userEvent.click(
      within(screen.getByLabelText("Run tasks")).getByRole("button", {
        name: /task-2/,
      }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "worktree missing",
    );
    expect(screen.queryByText("+after")).not.toBeInTheDocument();
  });
});
