import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { AgentUsage, Backend, Tool } from "../api";
import Board from "./Board";

const tools: Tool[] = [
  { name: "claude", path: "/bin/claude", available: true },
  { name: "codex", path: "/bin/codex", available: true },
];
const later = (hours: number) =>
  new Date(Date.now() + hours * 3_600_000).toISOString();
const usage: AgentUsage[] = [
  {
    tool: "claude",
    limits: [
      {
        label: "5-hour session",
        window: "session",
        percent: 59,
        resetsAt: later(2),
        level: "normal",
      },
      {
        label: "Weekly · all models",
        window: "weekly",
        percent: 88,
        resetsAt: later(8),
        level: "warning",
      },
    ],
    readAt: new Date().toISOString(),
  },
  {
    tool: "codex",
    limits: [],
    reason: "Codex did not report its limits within 25 seconds.",
    readAt: new Date().toISOString(),
  },
];
beforeEach(() => {
  window.go = {
    main: {
      App: { ListRuns: async () => [] } as unknown as Backend,
    },
  };
});

test("capacity cards show the limit closest to running out and open Usage limits", async () => {
  const open = vi.fn();
  render(
    <Board
      tools={tools}
      chats={[]}
      onOpen={vi.fn()}
      onOpenRun={vi.fn()}
      usage={usage}
      onUsage={open}
    />,
  );
  const limit = screen.getByRole("button", {
    name: /^Claude Code: 88% of week · resets .+\. Open usage limits$/,
  });
  expect(limit).toHaveClass("warning");
  expect(limit).toHaveAttribute("title", "Weekly · all models: 88% used");
  // An agent whose limits could not be read shows no bar, never 0%.
  expect(screen.queryByRole("button", { name: /^Codex:/ })).toBeNull();
  expect(screen.queryByText(/0% of/)).toBeNull();
  await userEvent.click(limit);
  expect(open).toHaveBeenCalled();
});

test("a reached limit says so with its reset time", () => {
  render(
    <Board
      tools={tools}
      chats={[]}
      onOpen={vi.fn()}
      onOpenRun={vi.fn()}
      usage={[
        {
          tool: "codex",
          blocked: true,
          limits: [
            {
              label: "Weekly",
              window: "weekly",
              percent: 100,
              resetsAt: later(30),
              level: "reached",
            },
          ],
          readAt: new Date().toISOString(),
        },
      ]}
    />,
  );
  expect(
    screen.getByRole("button", {
      name: /^Codex: Limit reached · resets .+\. Open usage limits$/,
    }),
  ).toHaveClass("reached");
  // Before limits are read, nothing is shown for the other agent.
  expect(screen.queryByRole("button", { name: /^Claude Code:/ })).toBeNull();
});
