import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ChatWorkspace from "./ChatWorkspace";
import type { Backend, ChatState, ReviewDraft } from "./api";

const initial: ChatState = {
  id: "chat",
  tool: "codex",
  project: "/project",
  sessionId: "native",
  sourceKey: "",
  title: "Fix checkout",
  status: "idle",
  error: "",
  messages: [],
  approvals: [],
  updatedAt: "2026-10-04T10:00:00Z",
  shortened: false,
  version: 1,
};
const draft: ReviewDraft = {
  id: "draft",
  sourceId: "chat",
  tool: "claude",
  created: initial.updatedAt,
  snapshot: {
    root: "/project",
    base: "abcdef123456",
    hash: "abcdef0123456789",
    diff: "",
    warnings: [],
    files: [{ path: "app.txt", patch: "@@ -1 +1 @@\n-before\n+after\n" }],
  },
};
let state: ChatState;
let backend: Backend;
const handlers = {
  onOpen: vi.fn(),
  onSelect: vi.fn(),
  onClose: vi.fn(),
  onState: vi.fn(),
};
beforeEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
  window.localStorage.clear();
  state = structuredClone(initial);
  backend = {
    ChatModels: vi
      .fn()
      .mockResolvedValue({ models: [], defaultModel: "", defaultEffort: "" }),
    SetChatSettings: vi.fn().mockResolvedValue(undefined),
    ReadChat: vi.fn(async (_id: string, version: number) => ({
      version: state.version,
      state: version === state.version ? null : structuredClone(state),
    })),
    SendChat: vi.fn().mockResolvedValue(undefined),
    StopChat: vi.fn().mockResolvedValue(undefined),
    CloseChat: vi.fn().mockResolvedValue(undefined),
    AnswerChat: vi.fn().mockResolvedValue(undefined),
    PrepareReview: vi.fn().mockResolvedValue(draft),
    ChatChanges: vi.fn().mockResolvedValue({
      available: true,
      files: [
        { path: "app.txt", status: "M", added: 1, removed: 1 },
        { path: "notes.md", status: "?", added: 4, removed: 0 },
      ],
      added: 5,
      removed: 1,
    }),
    ChatDiff: vi.fn().mockResolvedValue(draft.snapshot),
    StartReview: vi
      .fn()
      .mockResolvedValue({ ...initial, id: "review", tool: "claude" }),
    SendReviewFindings: vi.fn().mockResolvedValue("chat"),
  } as unknown as Backend;
  window.go = { main: { App: backend } };
});
function show() {
  return render(
    <ChatWorkspace chat={initial} active reviewerAvailable {...handlers} />,
  );
}
describe("graphical conversations", () => {
  it("lists changed files beside the conversation and opens the diff on request", async () => {
    show();
    const panel = await screen.findByRole("complementary", {
      name: "Conversation details",
    });
    expect(await within(panel).findByText("2 files")).toBeVisible();
    expect(within(panel).getByText("+5")).toBeVisible();
    expect(backend.ChatDiff).not.toHaveBeenCalled();
    await userEvent.click(
      within(panel).getByRole("button", {
        name: /Modified.*app\.txt|app\.txt/,
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Uncommitted changes",
    });
    expect(backend.ChatDiff).toHaveBeenCalledWith("chat");
    expect(await within(dialog).findByText(/after/)).toBeVisible();
    expect(within(dialog).getByText(/read-only/)).toBeVisible();
  });
  it("opens details as a drawer in a narrow window and returns focus on Escape", async () => {
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: (query: string) => ({
        matches: query.includes("max-width: 1199px"),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    });
    try {
      show();
      const toggle = await screen.findByRole("button", { name: /^Changes/ });
      await waitFor(() => expect(toggle).toHaveTextContent("2"));
      expect(toggle).toHaveAttribute("aria-pressed", "false");
      expect(
        screen.queryByRole("complementary", { name: "Conversation details" }),
      ).not.toBeInTheDocument();
      fireEvent.keyDown(document, { key: "\\", metaKey: true });
      const drawer = await screen.findByRole("complementary", {
        name: "Conversation details",
      });
      expect(toggle).toHaveAttribute("aria-pressed", "true");
      const close = within(drawer).getByRole("button", {
        name: "Close details",
      });
      expect(close).toHaveFocus();
      fireEvent.keyDown(close, { key: "Escape" });
      expect(
        screen.queryByRole("complementary", { name: "Conversation details" }),
      ).not.toBeInTheDocument();
      expect(toggle).toHaveFocus();
    } finally {
      Reflect.deleteProperty(window, "matchMedia");
    }
  });
  it("applies partial updates and reads everything again after a missed one", async () => {
    const text = (id: string, value: string) => ({
      id,
      role: "assistant",
      kind: "text",
      text: value,
    });
    const replies = [
      { version: 1, state: { ...initial, messages: [text("a", "First")] } },
      {
        version: 2,
        partial: true,
        first: "a",
        count: 2,
        state: { ...initial, version: 2, messages: [text("b", "Second")] },
      },
      // Claims three messages, but the reader only has two: out of step.
      {
        version: 4,
        partial: true,
        first: "a",
        count: 4,
        state: { ...initial, version: 4, messages: [text("d", "Fourth")] },
      },
      {
        version: 4,
        state: {
          ...initial,
          version: 4,
          messages: [
            text("a", "First"),
            text("b", "Second"),
            text("c", "Third"),
            text("d", "Fourth"),
          ],
        },
      },
    ];
    const read = vi.fn(
      async (_id: string, _version: number) =>
        replies.shift() ?? { version: 4, state: null },
    );
    backend.ReadChat = read;
    show();
    await screen.findByText("Fourth");
    expect(screen.getByText("First")).toBeVisible();
    expect(screen.getByText("Third")).toBeVisible();
    expect(read.mock.calls.slice(0, 4).map((call) => call[1])).toEqual([
      -1, 1, 2, -1,
    ]);
  });
  it("respects reading position and offers a jump back to the latest reply", async () => {
    const view = show();
    const messages = view.container.querySelector(
      ".chat-messages",
    ) as HTMLDivElement;
    Object.defineProperties(messages, {
      scrollHeight: { configurable: true, value: 1400 },
      clientHeight: { configurable: true, value: 300 },
    });
    messages.scrollTop = 100;
    fireEvent.scroll(messages);
    expect(
      screen.getByRole("button", { name: "Jump to latest ↓" }),
    ).toBeVisible();
    state = {
      ...state,
      version: 2,
      messages: [
        {
          id: "later",
          role: "assistant",
          kind: "text",
          text: "A later response",
        },
      ],
    };
    await screen.findByText("A later response");
    expect(messages.scrollTop).toBe(100);
    await userEvent.click(
      screen.getByRole("button", { name: "Jump to latest ↓" }),
    );
    expect(messages.scrollTop).toBe(1400);
    expect(
      screen.queryByRole("button", { name: "Jump to latest ↓" }),
    ).not.toBeInTheDocument();
  });
  it("recovers multiline drafts across remounts and keeps conversations separate", async () => {
    const view = show();
    const text = "  Investigate checkout\nKeep the existing API.  ";
    fireEvent.change(screen.getByLabelText("Message agent"), {
      target: { value: text },
    });
    expect(screen.getByRole("status")).toHaveTextContent(
      "Draft saved on this Mac",
    );
    view.unmount();
    state = { ...initial, id: "another-chat" };
    const other = render(
      <ChatWorkspace chat={state} active reviewerAvailable {...handlers} />,
    );
    expect(screen.getByLabelText("Message agent")).toHaveValue("");
    other.unmount();
    state = structuredClone(initial);
    show();
    expect(screen.getByLabelText("Message agent")).toHaveValue(text);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Draft restored from this Mac",
    );
    expect(backend.SendChat).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    expect(screen.getByLabelText("Message agent")).toHaveValue("");
    expect(window.localStorage.getItem("ais:chat-draft:v1:chat")).toBeNull();
  });
  it("retains a failed send on disk and clears only after a successful send", async () => {
    backend.SendChat = vi
      .fn()
      .mockRejectedValueOnce(new Error("Offline"))
      .mockResolvedValue(undefined);
    const view = show();
    fireEvent.change(screen.getByLabelText("Message agent"), {
      target: { value: "Keep my request" },
    });
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("Offline");
    view.unmount();
    show();
    expect(screen.getByLabelText("Message agent")).toHaveValue(
      "Keep my request",
    );
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Message agent")).toHaveValue(""),
    );
    expect(window.localStorage.getItem("ais:chat-draft:v1:chat")).toBeNull();
  });
  it("keeps writing and sending available when draft storage fails", async () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw Error("Unavailable");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw Error("Full");
    });
    show();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Draft storage is unavailable",
    );
    fireEvent.change(screen.getByLabelText("Message agent"), {
      target: { value: "Still send this" },
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Could not save this draft",
    );
    expect(screen.getByLabelText("Message agent")).toHaveValue(
      "Still send this",
    );
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(backend.SendChat).toHaveBeenCalledWith("chat", "Still send this");
  });
  it("warns when a sent draft cannot be removed from storage", async () => {
    show();
    fireEvent.change(screen.getByLabelText("Message agent"), {
      target: { value: "Saved request" },
    });
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw Error("Unavailable");
    });
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "It may reappear",
    );
    expect(screen.getByLabelText("Message agent")).toHaveValue("");
  });
  it("does not send during IME composition or for Shift+Enter", async () => {
    show();
    const input = screen.getByLabelText("Message agent");
    fireEvent.change(input, { target: { value: "入力中" } });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(backend.SendChat).not.toHaveBeenCalled();
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(backend.SendChat).toHaveBeenCalledWith("chat", "入力中"),
    );
  });
  it("sends a message once and uses current server state after a late acknowledgement", async () => {
    let finish!: () => void;
    backend.SendChat = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    show();
    await userEvent.type(
      screen.getByLabelText("Message agent"),
      "Explain checkout",
    );
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(backend.SendChat).toHaveBeenCalledTimes(1);
    state = {
      ...state,
      version: 2,
      messages: [
        {
          id: "reply",
          role: "assistant",
          kind: "text",
          text: "Checkout explained",
        },
      ],
    };
    await screen.findByText("Checkout explained");
    await act(async () => finish());
    expect(screen.getByLabelText("Message agent")).toHaveValue("");
    expect(
      screen.queryByRole("button", { name: "Stop response" }),
    ).not.toBeInTheDocument();
  });
  it("preserves a failed message and shows the failure", async () => {
    backend.SendChat = vi
      .fn()
      .mockRejectedValue(new Error("Agent unavailable"));
    show();
    await userEvent.type(
      screen.getByLabelText("Message agent"),
      "Keep this draft",
    );
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Agent unavailable",
    );
    expect(screen.getByLabelText("Message agent")).toHaveValue(
      "Keep this draft",
    );
  });
  it("shows streamed text, expandable tool output and stops the current response", async () => {
    state = {
      ...state,
      status: "running",
      messages: [
        {
          id: "text",
          role: "assistant",
          kind: "text",
          text: "Checking checkout",
        },
        {
          id: "tool",
          role: "assistant",
          kind: "tool",
          name: "Run tests",
          text: "2 tests passed",
          status: "completed",
        },
      ],
    };
    show();
    await screen.findByText("Checking checkout");
    expect(
      screen.getByText("2 tests passed").closest("details"),
    ).not.toHaveAttribute("open");
    await userEvent.click(screen.getByText("Run tests"));
    expect(screen.getByText("2 tests passed")).toBeVisible();
    await userEvent.click(
      screen.getByRole("button", { name: "Stop response" }),
    );
    // Stopping asks first, says what is kept, and focuses the safe choice.
    const confirm = await screen.findByRole("dialog", { name: "Stop Codex?" });
    expect(within(confirm).getByText(/already edited stay/)).toBeVisible();
    expect(
      within(confirm).getByRole("button", { name: "Keep running" }),
    ).toHaveFocus();
    expect(backend.StopChat).not.toHaveBeenCalled();
    await userEvent.click(
      within(confirm).getByRole("button", { name: "Stop response" }),
    );
    expect(backend.StopChat).toHaveBeenCalledWith("chat");
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Stop Codex?" }),
      ).not.toBeInTheDocument(),
    );
  });
  it("shows the command and answers from the keyboard only while the card has focus", async () => {
    state = {
      ...state,
      status: "waiting",
      approvals: [
        {
          id: "approval",
          title: "Run checkout tests",
          detail: "npm test -- checkout",
          questions: [],
        },
      ],
    };
    show();
    const card = await screen.findByRole("region", {
      name: "Run checkout tests",
    });
    const command = within(card).getByLabelText("Action details");
    expect(command).toHaveTextContent("npm test -- checkout");
    // Enter in the message box must not approve anything.
    fireEvent.keyDown(screen.getByLabelText("Message agent"), {
      key: "Escape",
    });
    expect(backend.AnswerChat).not.toHaveBeenCalled();
    command.focus();
    fireEvent.keyDown(command, { key: "Enter" });
    expect(backend.AnswerChat).toHaveBeenCalledWith(
      "chat",
      "approval",
      true,
      {},
    );
  });
  it("denies an approval with Escape inside the card", async () => {
    state = {
      ...state,
      status: "waiting",
      approvals: [
        {
          id: "approval",
          title: "Delete build output",
          detail: "rm -rf dist",
          questions: [],
        },
      ],
    };
    show();
    const card = await screen.findByRole("region", {
      name: "Delete build output",
    });
    fireEvent.keyDown(
      within(card).getByRole("button", { name: "Allow once" }),
      {
        key: "Escape",
      },
    );
    expect(backend.AnswerChat).toHaveBeenCalledWith(
      "chat",
      "approval",
      false,
      {},
    );
  });
  it("requires question answers and submits only one approval decision", async () => {
    state = {
      ...state,
      status: "waiting",
      approvals: [
        {
          id: "approval",
          title: "Choose a target",
          detail: "Details",
          questions: [
            {
              id: "target",
              text: "Which target?",
              options: ["Local", "Preview"],
            },
          ],
        },
      ],
    };
    show();
    const card = await screen.findByRole("region", { name: "Choose a target" });
    expect(
      within(card).getByRole("button", { name: "Send answers" }),
    ).toBeDisabled();
    await userEvent.selectOptions(
      within(card).getByLabelText("Options for Which target?"),
      "Local",
    );
    fireEvent.click(within(card).getByRole("button", { name: "Send answers" }));
    fireEvent.click(within(card).getByRole("button", { name: "Sending…" }));
    await waitFor(() => expect(backend.AnswerChat).toHaveBeenCalledTimes(1));
    expect(backend.AnswerChat).toHaveBeenCalledWith("chat", "approval", true, {
      target: "Local",
    });
  });
  it("shows the exact diff before starting the other agent and surfaces a stale diff", async () => {
    backend.StartReview = vi
      .fn()
      .mockRejectedValue(new Error("working tree changed since preview"));
    show();
    await userEvent.click(
      screen.getByRole("button", { name: "Review with Claude Code" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("+after")).toBeVisible();
    expect(backend.StartReview).not.toHaveBeenCalled();
    await userEvent.click(
      within(dialog).getByRole("button", {
        name: "Send snapshot to Claude Code",
      }),
    );
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "working tree changed",
    );
    expect(handlers.onOpen).not.toHaveBeenCalled();
  });
  it("links a finding to its diff line and sends only selected findings back", async () => {
    state = {
      ...state,
      id: "review",
      tool: "claude",
      review: {
        sourceId: "chat",
        snapshot: draft.snapshot,
        summary: "Check the result",
        complete: true,
        findings: [
          {
            id: "finding",
            priority: "P2",
            file: "app.txt",
            line: 1,
            title: "Handle failure",
            body: "Check before using the result.",
            sent: false,
          },
        ],
      },
    };
    render(
      <ChatWorkspace chat={state} active reviewerAvailable {...handlers} />,
    );
    await screen.findByText("Handle failure");
    await userEvent.click(screen.getByRole("button", { name: "app.txt:1" }));
    expect(screen.getByText("+after").parentElement).toHaveClass("focused");
    await userEvent.click(screen.getByLabelText("Select Handle failure"));
    await userEvent.click(
      screen.getByRole("button", { name: "Send 1 findings to Codex" }),
    );
    expect(backend.SendReviewFindings).toHaveBeenCalledWith("review", [
      "finding",
    ]);
    expect(handlers.onSelect).toHaveBeenCalledWith("chat");
  });
  it("renders provider output without active HTML or remote images", async () => {
    state = {
      ...state,
      messages: [
        {
          id: "unsafe",
          role: "assistant",
          kind: "text",
          text: "<script>alert(1)</script>\n\n![image](https://example.invalid/track)\n\n[link](javascript:alert(1))",
        },
      ],
    };
    show();
    await screen.findByText("[Image omitted]");
    expect(
      document.querySelector(
        ".chat-messages script, .chat-messages img, .chat-messages a",
      ),
    ).toBeNull();
  });
});

it("does not send a message until model preferences finish saving", async () => {
  let finish!: () => void;
  backend.ChatModels = vi.fn().mockResolvedValue({
    models: [
      {
        id: "model-a",
        name: "Model A",
        description: "",
        efforts: ["high"],
        defaultEffort: "high",
      },
    ],
    defaultModel: "model-a",
    defaultEffort: "high",
  });
  backend.SetChatSettings = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
  );
  show();
  await screen.findByRole("option", { name: "Model A" });
  await userEvent.type(
    screen.getByLabelText("Message agent"),
    "Use my selected model",
  );
  fireEvent.change(screen.getByLabelText("Model"), {
    target: { value: "model-a" },
  });
  expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  fireEvent.keyDown(screen.getByLabelText("Message agent"), { key: "Enter" });
  expect(backend.SendChat).not.toHaveBeenCalled();
  await act(async () => finish());
  await userEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(backend.SendChat).toHaveBeenCalledTimes(1);
});
it("shows the reported response model independently of the selected model", async () => {
  state = {
    ...state,
    settings: { model: "requested-model", effort: "high" },
    messages: [
      {
        id: "answer",
        role: "assistant",
        kind: "text",
        text: "Answer",
        model: "reported-model",
      },
    ],
  };
  show();
  expect(await screen.findByText("reported-model")).toBeVisible();
});

it("shows the reported reviewer model after structured review text is hidden", async () => {
  state = {
    ...state,
    id: "review",
    tool: "claude",
    activeModel: "reviewer-reported-model",
    review: {
      sourceId: "chat",
      snapshot: draft.snapshot,
      summary: "No issues",
      complete: true,
      findings: [],
    },
  };
  render(<ChatWorkspace chat={state} active reviewerAvailable {...handlers} />);
  expect(await screen.findByText(/reviewer-reported-model/)).toBeVisible();
});
