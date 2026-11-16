import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { expect, test, vi } from "vitest";
import type { ChatMessage } from "../api";
import FindBar from "./FindBar";
import MessageList, { messageTime } from "./MessageList";
import { SafeMarkdown } from "../ui/SafeMarkdown";

// Agents record tool input as indented JSON.
const json = (value: object) => JSON.stringify(value, null, 2);
const text = (id: string, role: string, value: string, at?: string) =>
  ({ id, role, kind: "text", text: value, at }) as ChatMessage;
// Claude Code shapes: an edit with old and new text, a command, a read.
const edit = (id: string, file: string, from: string, to: string) =>
  ({
    id,
    role: "assistant",
    kind: "tool",
    name: "Edit",
    status: "completed",
    text: json({
      file_path: `/p/${file}`,
      old_string: from,
      new_string: to,
    }),
  }) as ChatMessage;
const run = (id: string, command: string, status = "completed") =>
  ({
    id,
    role: "assistant",
    kind: "tool",
    name: "Bash",
    status,
    text: json({ command }),
  }) as ChatMessage;
const read = (id: string, file: string) =>
  ({
    id,
    role: "assistant",
    kind: "tool",
    name: "Read",
    status: "completed",
    text: json({ file_path: `/p/${file}` }),
  }) as ChatMessage;
const turn = [
  text("u", "user", "Make retries safe"),
  read("r", "src/retry.ts"),
  edit("e1", "src/retry.ts", "a", "b"),
  run("c1", "npm test", "failed"),
  edit("e2", "src/checkout.ts", "charge()", "retry(charge)"),
  run("c2", "npm test"),
  text("a", "assistant", "Done."),
];

test("a finished turn's long tool activity folds to a summary", async () => {
  render(<MessageList tool="claude" project="/p" messages={turn} />);
  const summary = screen.getByRole("button", {
    name: "5 steps · 2 edits · 2 commands · 1 read 1 failed",
  });
  expect(summary).toHaveAttribute("aria-expanded", "false");
  const group = screen.getByRole("group", { name: "Tool activity" });
  expect(within(group).getByText("src/checkout.ts")).not.toBeVisible();
  await userEvent.click(summary);
  expect(summary).toHaveAttribute("aria-expanded", "true");
  expect(within(group).getByText("src/checkout.ts")).toBeVisible();
  // Only the latest edit shows its diff until you choose otherwise.
  const steps = group.querySelectorAll("details");
  expect([...steps].map((step) => step.open)).toEqual([
    false,
    false,
    false,
    true,
    false,
  ]);
});

test("the running turn's activity stays open, and short groups never fold", () => {
  const { rerender } = render(
    <MessageList tool="claude" project="/p" messages={turn.slice(0, 6)} busy />,
  );
  expect(screen.getByRole("button", { name: /^5 steps/ })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  rerender(
    <MessageList
      tool="claude"
      project="/p"
      messages={[turn[0], turn[1], turn[2], turn[6]]}
    />,
  );
  expect(screen.queryByRole("button", { name: /steps/ })).toBeNull();
  // The read and the edit of src/retry.ts are both shown.
  for (const target of screen.getAllByText("src/retry.ts", {
    selector: "strong",
  }))
    expect(target).toBeVisible();
});

test("long messages and long code fold, short ones don't", async () => {
  const log = Array.from({ length: 30 }, (_, i) => `line ${i + 1}`).join("\n");
  render(
    <MessageList
      tool="claude"
      messages={[text("long", "user", log), text("short", "user", "Hi")]}
    />,
  );
  const more = screen.getByRole("button", { name: "Show more" });
  expect(more).toHaveAttribute("aria-expanded", "false");
  expect(screen.getAllByRole("button", { name: /Show/ })).toHaveLength(1);
  await userEvent.click(more);
  expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );

  const code =
    "```ts\n" +
    Array.from({ length: 40 }, (_, i) => `x${i};`).join("\n") +
    "\n```";
  const long = render(<SafeMarkdown text={code} />);
  expect(
    screen.getByRole("button", { name: "Show all 40 lines" }),
  ).toBeVisible();
  long.unmount();
  render(<SafeMarkdown text={"```\nshort\n```"} />);
  expect(screen.queryByRole("button", { name: /Show all/ })).toBeNull();
});

test("messages show when they arrived, and yours can be reused", async () => {
  const reuse = vi.fn();
  const at = new Date(2026, 10, 16, 9, 5).toISOString();
  render(
    <MessageList
      tool="codex"
      onReuse={reuse}
      messages={[
        text("u", "user", "Add a test", at),
        text("a", "assistant", "Added.", at),
        text("old", "user", "Saved before times were kept"),
      ]}
    />,
  );
  const times = screen.getAllByText(messageTime(at));
  expect(times).toHaveLength(2);
  expect(times[0]).toHaveAttribute("datetime", at);
  // Only your own messages can be reused.
  const buttons = screen.getAllByRole("button", { name: "Reuse this message" });
  expect(buttons).toHaveLength(2);
  await userEvent.click(buttons[0]);
  expect(reuse).toHaveBeenCalledWith("Add a test");
});

test("message times read naturally", () => {
  const now = new Date(2026, 10, 16, 18, 0);
  const clock = (d: Date) =>
    d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const today = new Date(2026, 10, 16, 9, 5);
  expect(messageTime(today.toISOString(), now)).toBe(clock(today));
  const monday = new Date(2026, 10, 14, 9, 5);
  expect(messageTime(monday.toISOString(), now)).toMatch(/^\S+ \d/);
  const lastYear = new Date(2025, 0, 2, 9, 5);
  expect(messageTime(lastYear.toISOString(), now)).toContain("2025");
  expect(messageTime("not a time", now)).toBe("");
});

function Findable({ messages }: { messages: ChatMessage[] }) {
  const container = useRef<HTMLDivElement>(null);
  return (
    <>
      <FindBar container={container} version={1} onClose={vi.fn()} />
      <div ref={container}>
        <MessageList tool="claude" project="/p" messages={messages} />
      </div>
    </>
  );
}

test("find opens folded activity and folded text to show a match", async () => {
  const log = Array.from({ length: 30 }, (_, i) => `entry ${i}`).join("\n");
  render(<Findable messages={[text("log", "user", log), ...turn.slice(1)]} />);
  expect(screen.getByRole("button", { name: /^5 steps/ })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  await userEvent.type(
    screen.getByLabelText("Find in conversation"),
    "checkout.ts",
  );
  expect(await screen.findByText("1 of 1")).toBeVisible();
  expect(screen.getByRole("button", { name: /^5 steps/ })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  const field = screen.getByLabelText("Find in conversation");
  await userEvent.clear(field);
  await userEvent.type(field, "entry 29");
  expect(await screen.findByText("1 of 1")).toBeVisible();
  expect(screen.getByRole("button", { name: "Show less" })).toBeVisible();
});
