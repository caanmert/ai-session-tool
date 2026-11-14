import { render, screen, within } from "@testing-library/react";
import { expect, test } from "vitest";
import type { ChatMessage } from "../api";
import MessageList from "./MessageList";

const text = (id: string, role: string, value: string): ChatMessage => ({
  id,
  role,
  kind: "text",
  text: value,
});
const tool = (id: string, name: string, status: string): ChatMessage => ({
  id,
  role: "assistant",
  kind: "tool",
  name,
  status,
  text: `${name} output`,
});

test("consecutive tool calls share one block and state their status in words", () => {
  render(
    <MessageList
      tool="claude"
      messages={[
        text("u", "user", "Rotate the keys"),
        tool("t1", "Read", "completed"),
        tool("t2", "npm test auth", "failed"),
        text("a", "assistant", "One test fails."),
        tool("t3", "Edit", "running"),
      ]}
    />,
  );
  const groups = screen.getAllByRole("group", { name: "Tool activity" });
  expect(groups).toHaveLength(2);
  expect(within(groups[0]).getByText("Read")).toBeVisible();
  expect(within(groups[0]).getByText("Done")).toBeVisible();
  // A command line is shown as code; a failure is named, not only colored.
  expect(within(groups[0]).getByText("npm test auth")).toHaveClass("command");
  expect(within(groups[0]).getByText("Failed")).toHaveClass("failed");
  expect(within(groups[1]).getByText("Running")).toBeVisible();
  expect(screen.getByText("Rotate the keys")).toBeVisible();
  expect(screen.getByText("Claude Code")).toBeVisible();
});
