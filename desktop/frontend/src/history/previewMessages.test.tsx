import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";
import type { Message } from "../api";
import MessageList from "../chat/MessageList";
import { previewMessages } from "./previewMessages";

// The shapes the CLI's transcript reader produces for Claude Code.
const saved: Message[] = [
  {
    role: "user",
    kind: "text",
    text: "Fix the retries",
    time: "2026-11-16T08:00:00Z",
  },
  { role: "assistant", kind: "thinking", text: "Look at checkout first." },
  {
    role: "assistant",
    kind: "tool_use",
    toolName: "Read",
    text: "/p/src/checkout.ts",
  },
  { role: "assistant", kind: "tool_use", toolName: "Bash", text: "npm test" },
  { role: "user", kind: "tool_result", text: "export function checkout() {}" },
  { role: "user", kind: "tool_result", text: "3 tests passed" },
  { role: "user", kind: "tool_result", text: "late output" },
  {
    role: "assistant",
    kind: "tool_use",
    toolName: "Grep",
    text: "client.charge",
  },
  {
    role: "assistant",
    kind: "text",
    text: "Done.",
    time: "2026-11-16T08:02:00Z",
  },
];

test("saved messages become the conversation's shape, results paired in order", () => {
  const messages = previewMessages(saved);
  expect(
    messages.map((m) => [m.kind, m.name || m.role, m.status || ""]),
  ).toEqual([
    ["text", "user", ""],
    ["tool", "Reasoning", "completed"],
    ["tool", "Read", "completed"],
    ["tool", "Bash", "completed"],
    ["tool", "Output", "completed"],
    ["tool", "Grep", ""],
    ["text", "assistant", ""],
  ]);
  expect(messages[0].at).toBe("2026-11-16T08:00:00Z");
  // The first result belongs to the first call, the second to the second.
  expect(messages[2].text).toBe(
    '{\n  "file_path": "/p/src/checkout.ts"\n}\n\nexport function checkout() {}',
  );
  expect(messages[3].text).toBe(
    '{\n  "command": "npm test"\n}\n\n3 tests passed',
  );
  expect(messages[4].text).toBe("late output");
  expect(new Set(messages.map((m) => m.id)).size).toBe(messages.length);
  // A kind added later is shown rather than dropped.
  const unknown = previewMessages([
    { role: "assistant", kind: "image" as Message["kind"], text: "chart.png" },
  ]);
  expect(unknown).toEqual([
    expect.objectContaining({
      kind: "tool",
      name: "Output",
      text: "chart.png",
    }),
  ]);
});

test("History draws saved steps as readable rows, folded once there are many", async () => {
  render(
    <MessageList
      tool="claude"
      project="/p"
      messages={previewMessages(saved)}
    />,
  );
  // Your messages say "You", as in open conversations.
  expect(screen.getAllByText("You").length).toBeGreaterThan(0);
  const summary = screen.getByRole("button", {
    name: /^5 steps · 1 command · 1 read · 1 search/,
  });
  expect(summary).toHaveAttribute("aria-expanded", "false");
  await userEvent.click(summary);
  const group = screen.getByRole("group", { name: "Tool activity" });
  // Paths are shown relative to the project, commands as they ran.
  expect(within(group).getByText("src/checkout.ts")).toBeVisible();
  expect(within(group).getByText("npm test")).toBeVisible();
  expect(within(group).getByText("client.charge")).toBeVisible();
  expect(screen.queryByText("Tool output")).toBeNull();
});
