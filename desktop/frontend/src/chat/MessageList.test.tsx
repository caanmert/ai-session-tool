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
