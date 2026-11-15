import { expect, test } from "vitest";
import type { ChatState } from "../api";
import {
  conversationMarkdown,
  conversationStatus,
  sidebarOrder,
} from "./conversation";

const chat = (patch: Partial<ChatState>) =>
  ({
    id: "c",
    tool: "claude",
    project: "/work/checkout",
    title: "Rotate keys",
    status: "idle",
    approvals: [],
    messages: [],
    ...patch,
  }) as ChatState;

test("says what a waiting conversation waits for", () => {
  const approval = { id: "a", title: "Run", detail: "npm test", questions: [] };
  const question = {
    ...approval,
    questions: [{ id: "q", text: "Which?", options: [] }],
  };
  expect(conversationStatus(chat({ status: "running" }))).toBe("Working");
  expect(
    conversationStatus(chat({ status: "waiting", approvals: [approval] })),
  ).toBe("Needs approval");
  expect(
    conversationStatus(chat({ status: "waiting", approvals: [question] })),
  ).toBe("Has a question");
  expect(conversationStatus(chat({ status: "waiting" }))).toBe(
    "Needs your input",
  );
  expect(conversationStatus(chat({ status: "error" }))).toBe("Needs attention");
});

test("orders open conversations as the sidebar groups them", () => {
  const { ordered } = sidebarOrder([
    chat({ id: "1", project: "/a" }),
    chat({ id: "2", project: "/b" }),
    chat({ id: "3", project: "/a" }),
  ]);
  expect(ordered.map((c) => c.id)).toEqual(["1", "3", "2"]);
});

test("writes a conversation as Markdown without tool output", () => {
  const markdown = conversationMarkdown(
    chat({
      messages: [
        { id: "u", role: "user", kind: "text", text: "Rotate the keys." },
        {
          id: "t1",
          role: "assistant",
          kind: "tool",
          name: "Read",
          status: "completed",
          text: "SECRET FILE CONTENTS",
        },
        {
          id: "t2",
          role: "assistant",
          kind: "tool",
          name: "npm test",
          status: "failed",
          text: "long output",
        },
        { id: "a", role: "assistant", kind: "text", text: "One test fails." },
      ],
    }),
  );
  expect(markdown).toBe(
    [
      "# Rotate keys",
      "Claude Code · /work/checkout",
      "**You**\n\nRotate the keys.",
      "- `Read` (completed)\n- `npm test` (failed)",
      "**Claude Code**\n\nOne test fails.",
    ].join("\n\n") + "\n",
  );
  expect(markdown).not.toContain("SECRET");
});
