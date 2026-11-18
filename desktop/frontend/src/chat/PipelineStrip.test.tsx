import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, ChatState, Pipeline } from "../api";
import PipelineStrip, { stepNote, stepStates } from "./PipelineStrip";

const base: Pipeline = {
  id: "pl-1",
  task: "Add retries",
  project: "/work/shop",
  planner: "claude",
  builder: "codex",
  planChat: "plan-chat",
  plan: "1. Add retries",
  step: "planned",
};
const chat = (id: string, step: "plan" | "build"): ChatState => ({
  id,
  tool: step === "plan" ? "claude" : "codex",
  project: "/work/shop",
  sessionId: "s",
  sourceKey: "",
  title: "Plan: Add retries",
  status: "idle",
  error: "",
  messages: [],
  approvals: [],
  updatedAt: "2026-11-18T10:00:00Z",
  shortened: false,
  version: 3,
  pipeline: {
    id: "pl-1",
    step,
    task: "Add retries",
    planner: "claude",
    builder: "codex",
  },
});
let backend: Record<string, ReturnType<typeof vi.fn>>;
let view: Pipeline;
beforeEach(() => {
  view = { ...base };
  backend = {
    PipelineView: vi.fn(async () => view),
    BuildPipeline: vi.fn().mockResolvedValue(chat("build-chat", "build")),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("works out each step's state and what it waits for", () => {
  expect(stepStates("planning")).toEqual(["now", "next", "next"]);
  expect(stepStates("built")).toEqual(["done", "done", "now"]);
  expect(stepStates("reviewed")).toEqual(["done", "done", "done"]);
  expect(stepNote({ ...base, step: "unplanned" })).toBe(
    "Planning ended without a plan. Ask Claude Code for one in the plan conversation.",
  );
  expect(stepNote({ ...base, step: "reviewed", findings: 1 })).toMatch(
    /^Claude Code found 1 thing to consider\. Send the ones you agree with to Codex/,
  );
  expect(stepNote({ ...base, step: "built", reviewChat: "r" })).toMatch(
    /did not finish/,
  );
});

test("the plan is read and edited before the other agent builds it", async () => {
  const onOpen = vi.fn();
  render(
    <PipelineStrip
      chat={chat("plan-chat", "plan")}
      active
      onSelect={vi.fn()}
      onOpen={onOpen}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Build with Codex…" }),
  );
  const editor = screen.getByRole("textbox", {
    name: /The plan Codex will build/,
  });
  expect(editor).toHaveValue("1. Add retries");
  expect(backend.BuildPipeline).not.toHaveBeenCalled();
  await userEvent.type(editor, "\n2. Keep the key");
  await userEvent.click(
    screen.getByRole("button", { name: "Start the build" }),
  );
  await waitFor(() =>
    expect(backend.BuildPipeline).toHaveBeenCalledWith(
      "pl-1",
      "1. Add retries\n2. Keep the key",
    ),
  );
  expect(onOpen).toHaveBeenCalledWith(
    expect.objectContaining({ id: "build-chat" }),
  );
});

test("links the steps and offers the review in the build", async () => {
  view = { ...base, step: "built", buildChat: "build-chat" };
  const onSelect = vi.fn();
  const onReview = vi.fn();
  const { unmount } = render(
    <PipelineStrip
      chat={chat("build-chat", "build")}
      active
      onSelect={onSelect}
      onOpen={vi.fn()}
      onReview={onReview}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Review with Claude Code…" }),
  );
  expect(onReview).toHaveBeenCalled();
  expect(screen.getByText("Build · Codex")).toHaveAttribute(
    "aria-current",
    "step",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Plan · Claude Code ↗" }),
  );
  expect(onSelect).toHaveBeenCalledWith("plan-chat");
  unmount();
  // From the plan conversation, the review happens in the build.
  render(
    <PipelineStrip
      chat={chat("plan-chat", "plan")}
      active
      onSelect={onSelect}
      onOpen={vi.fn()}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Open the build to review it" }),
  );
  expect(onSelect).toHaveBeenLastCalledWith("build-chat");
});

test("says when the task can't be read", async () => {
  backend.PipelineView = vi
    .fn()
    .mockRejectedValue(new Error("that task's plan conversation is gone"));
  render(
    <PipelineStrip
      chat={chat("build-chat", "build")}
      active
      onSelect={vi.fn()}
      onOpen={vi.fn()}
    />,
  );
  expect(await screen.findByRole("alert")).toHaveTextContent("is gone");
});
