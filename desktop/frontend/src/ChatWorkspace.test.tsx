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
  vi.clearAllMocks();
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
    StartReview: vi
      .fn()
      .mockResolvedValue({ ...initial, id: "review", tool: "claude" }),
    SendReviewFindings: vi.fn().mockResolvedValue("chat"),
  } as unknown as Backend;
  window.go = { main: { App: backend } };
});
function show() {
  render(
    <ChatWorkspace chat={initial} active reviewerAvailable {...handlers} />,
  );
}
describe("graphical conversations", () => {
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
    fireEvent.click(screen.getByRole("button", { name: "Send ↑" }));
    fireEvent.click(screen.getByRole("button", { name: "Send ↑" }));
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
    await userEvent.click(screen.getByRole("button", { name: "Send ↑" }));
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
    expect(backend.StopChat).toHaveBeenCalledWith("chat");
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
  expect(screen.getByRole("button", { name: "Send ↑" })).toBeDisabled();
  fireEvent.keyDown(screen.getByLabelText("Message agent"), { key: "Enter" });
  expect(backend.SendChat).not.toHaveBeenCalled();
  await act(async () => finish());
  await userEvent.click(screen.getByRole("button", { name: "Send ↑" }));
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
