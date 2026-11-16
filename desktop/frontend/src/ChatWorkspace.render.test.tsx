import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

test(
  "streaming into a long conversation renders only the changing message",
  { timeout: 20000 },
  async () => {
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
    // Drawing 300 messages can take longer than the default wait on a busy
    // machine; the assertions below are about what rendered, not how fast.
    await screen.findByText("Working on it", {}, { timeout: 10000 });
    // Only the latest 150 messages are drawn, each exactly once; the reply
    // once per update. The first 150 are never drawn.
    const saved = rendered.filter((text) => text.startsWith("Message "));
    expect(saved).toHaveLength(150);
    expect(new Set(saved).size).toBe(150);
    expect(saved).toContain("Message 299");
    expect(saved).not.toContain("Message 149");
    // React may batch updates that arrive together, so the reply renders at
    // most once per update and ends on the latest text.
    const reply = rendered.filter((text) => text.startsWith("Work"));
    expect(reply.length).toBeLessThanOrEqual(3);
    expect(reply.at(-1)).toBe("Working on it");
    // Earlier messages are added on request, without redrawing the others.
    rendered.length = 0;
    await userEvent.click(
      screen.getByRole("button", { name: /Show 150 earlier messages/ }),
    );
    await screen.findByText("Message 1", {}, { timeout: 10000 });
    const added = rendered.filter((text) => text.startsWith("Message "));
    expect(added).toHaveLength(150);
    expect(added).not.toContain("Message 299");
    expect(
      screen.getByRole("button", { name: /Show 1 earlier messages/ }),
    ).toBeVisible();
  },
);
