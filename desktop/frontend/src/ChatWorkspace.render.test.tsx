import { render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import ChatWorkspace from "./ChatWorkspace";
import type { Backend, ChatState, ChatUpdate } from "./api";

const rendered: string[] = [];
vi.mock("react-markdown", () => ({
  default: ({ children }: { children: string }) => {
    rendered.push(children);
    return <p>{children}</p>;
  },
}));

test("streaming into a long conversation renders only the changing message", async () => {
  const messages = Array.from({ length: 300 }, (_, i) => ({
    id: `m${i}`,
    role: i % 2 ? "assistant" : "user",
    kind: "text",
    text: `Message ${i}`,
  }));
  const chat = {
    id: "chat",
    tool: "codex",
    project: "/project",
    title: "Long conversation",
    status: "running",
    error: "",
    messages,
    approvals: [],
    shortened: false,
    version: 1,
    settings: { model: "", effort: "", configured: false },
    reviewerSettings: { model: "", effort: "", configured: false },
  } as unknown as ChatState;
  const streamed = (version: number, text: string): ChatUpdate => ({
    version,
    partial: true,
    first: "m0",
    count: 301,
    state: {
      ...chat,
      version,
      messages: [{ id: "reply", role: "assistant", kind: "text", text }],
    },
  });
  const replies: ChatUpdate[] = [
    { version: 1, state: chat },
    streamed(2, "Work"),
    streamed(3, "Working on"),
    streamed(4, "Working on it"),
  ];
  window.go = {
    main: {
      App: {
        ReadChat: async () => replies.shift() ?? { version: 4, state: null },
        ChatChanges: async () => ({
          available: true,
          files: [],
          added: 0,
          removed: 0,
        }),
        ChatModels: async () => ({
          models: [],
          defaultModel: "",
          defaultEffort: "",
        }),
      } as unknown as Backend,
    },
  };
  render(
    <ChatWorkspace
      chat={chat}
      active
      reviewerAvailable={false}
      onOpen={vi.fn()}
      onSelect={vi.fn()}
      onState={vi.fn()}
    />,
  );
  await screen.findByText("Working on it");
  // Each of the 300 saved messages is drawn once; the reply once per update.
  expect(rendered.filter((text) => text.startsWith("Message "))).toHaveLength(
    300,
  );
  // React may batch updates that arrive together, so the reply renders at
  // most once per update and ends on the latest text.
  const reply = rendered.filter((text) => text.startsWith("Work"));
  expect(reply.length).toBeLessThanOrEqual(3);
  expect(reply.at(-1)).toBe("Working on it");
});
