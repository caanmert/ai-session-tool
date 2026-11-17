import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
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
  // A command line is shown after the verb; a failure is named, not only colored.
  expect(within(groups[0]).getByText("Run")).toBeVisible();
  expect(within(groups[0]).getByText("npm test auth")).toBeVisible();
  expect(within(groups[0]).getByText("Failed")).toHaveClass("failed");
  expect(within(groups[1]).getByText("Running")).toBeVisible();
  expect(screen.getByText("Rotate the keys")).toBeVisible();
  expect(screen.getByText("Claude Code")).toBeVisible();
});

test("copies a message and confirms it", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  render(
    <MessageList
      tool="codex"
      messages={[text("a", "assistant", "Use **three** retries.")]}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Copy message" }));
  // The original Markdown is copied, not the rendered text.
  expect(writeText).toHaveBeenCalledWith("Use **three** retries.");
  expect(await screen.findByText("Copied")).toBeVisible();
});

test("an edit shows the file, line counts and the changed lines", () => {
  render(
    <MessageList
      tool="claude"
      project="/work/app"
      messages={[
        {
          id: "e",
          role: "assistant",
          kind: "tool",
          name: "Edit",
          status: "completed",
          text:
            JSON.stringify(
              {
                file_path: "/work/app/src/keys.ts",
                old_string: "return env.SECRET;",
                new_string: "const key = store.current();\nreturn key.secret;",
              },
              null,
              2,
            ) + "\n\nThe file was updated.",
        },
      ]}
    />,
  );
  const group = screen.getByRole("group", { name: "Tool activity" });
  expect(within(group).getByText("Edit")).toBeVisible();
  expect(within(group).getByText("src/keys.ts")).toBeVisible();
  expect(within(group).getByText("+2")).toBeVisible();
  expect(within(group).getByText("−1")).toBeVisible();
  const lines = within(group).getByRole("group", { name: "Changed lines" });
  expect(
    within(lines).getByText("return env.SECRET;").closest("div"),
  ).toHaveClass("del");
  expect(
    within(lines).getByText("return key.secret;").closest("div"),
  ).toHaveClass("add");
  // The raw JSON input is not shown; the tool's result still is.
  expect(within(group).queryByText(/old_string/)).not.toBeInTheDocument();
  expect(within(group).getByText("The file was updated.")).toBeInTheDocument();
});

test("each finished turn ends with how long it took", () => {
  const { container } = render(
    <MessageList
      tool="claude"
      messages={[
        {
          ...text("u1", "user", "Rotate the keys"),
          turn: { ms: 134_000, tokens: 45_600, costUsd: 0.42 },
        },
        tool("t1", "Read", "completed"),
        text("a1", "assistant", "Done."),
        { ...text("u2", "user", "Now the docs"), turn: { ms: 9_000 } },
        text("a2", "assistant", "Updated."),
        // Still running: no figures yet.
        text("u3", "user", "And the changelog"),
      ]}
    />,
  );
  const lines = [...container.querySelectorAll(".turn-stats")];
  expect(lines.map((line) => line.textContent)).toEqual([
    "Worked 2m 14s · 46k tokens",
    "Worked 9s",
  ]);
  // Each line ends its turn: after the reply, before the next message.
  expect(lines[0].previousElementSibling).toHaveTextContent("Done.");
  expect(lines[0].nextElementSibling).toHaveTextContent("Now the docs");
  expect(lines[0]).toHaveAttribute(
    "title",
    expect.stringContaining("Claude Code estimates $0.42 at API prices"),
  );
  expect(screen.getByText("Updated.")).toBeVisible();
});
