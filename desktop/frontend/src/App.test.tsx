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
import App from "./App";
import type { Backend, Preview, Snapshot, ChatState } from "./api";

vi.mock("./ChatWorkspace", () => ({
  chatBusy: (status: string) =>
    ["running", "waiting", "stopping"].includes(status),
  default: ({
    chat: session,
    active,
  }: {
    chat: ChatState;
    active: boolean;
  }) => (
    <section hidden={!active} aria-label="Chat conversation">
      {session.id}
    </section>
  ),
}));
const live: ChatState = {
  id: "live-one",
  tool: "claude",
  project: "/demo/docs",
  sessionId: "two",
  sourceKey: "claude/two",
  title: "Chat",
  status: "idle",
  error: "",
  messages: [],
  approvals: [],
  updatedAt: "2026-10-04T10:00:00Z",
  shortened: false,
  version: 1,
};
const snapshot: Snapshot = {
  sessions: [
    {
      key: "codex/one",
      id: "one",
      tool: "codex",
      title: "Retry webhook delivery",
      cwd: "/demo/api",
      project: "/demo/api",
      gitBranch: "feat/retries",
      updatedAt: "2026-10-04T10:00:00Z",
      userTurns: 1,
      assistantTurns: 1,
      usage: { input: 1200, output: 200, cacheRead: 0, cacheWrite: 0 },
    },
    {
      key: "claude/two",
      id: "two",
      tool: "claude",
      title: "Document installation",
      cwd: "/demo/docs",
      project: "/demo/docs",
      updatedAt: "2026-10-03T10:00:00Z",
      userTurns: 1,
      assistantTurns: 1,
      usage: { input: 100, output: 50, cacheRead: 0, cacheWrite: 0 },
    },
  ],
  projects: [
    {
      name: "api",
      path: "/demo/api",
      sessions: 1,
      available: true,
      recent: false,
    },
    {
      name: "docs",
      path: "/demo/docs",
      sessions: 1,
      available: true,
      recent: false,
    },
  ],
  tools: [
    { name: "codex", path: "/bin/codex", available: true },
    { name: "claude", path: "/bin/claude", available: true },
  ],
  warnings: [],
};
let backend: Backend;
beforeEach(() => {
  backend = {
    ChatModels: vi
      .fn()
      .mockResolvedValue({ models: [], defaultModel: "", defaultEffort: "" }),
    SetChatSettings: vi.fn().mockResolvedValue(undefined),
    Refresh: vi.fn().mockResolvedValue(snapshot),
    Search: vi.fn().mockResolvedValue(["codex/one"]),
    Transcript: vi.fn().mockResolvedValue({
      messages: [
        { role: "assistant", kind: "text", text: "**Ready** to continue." },
      ],
      truncated: false,
    }),
    PickProject: vi.fn().mockResolvedValue(""),
    NewChat: vi.fn().mockResolvedValue(live),
    ResumeChat: vi.fn().mockResolvedValue(live),
    ListChats: vi.fn().mockResolvedValue([]),
    NewSession: vi.fn(),
    ResumeSession: vi.fn(),
    LiveSessions: vi.fn(),
    ReadChat: vi.fn(),
    SendChat: vi.fn(),
    StopChat: vi.fn(),
    CloseChat: vi.fn(),
    AnswerChat: vi.fn(),
    PrepareReview: vi.fn(),
    StartReview: vi.fn(),
    SendReviewFindings: vi.fn(),
    ReadTerminal: vi.fn(),
    WriteTerminal: vi.fn(),
    ResizeTerminal: vi.fn(),
    StopTerminal: vi.fn(),
    CloseTerminal: vi.fn(),
  };
  window.go = { main: { App: backend } };
});
describe("desktop workflows", () => {
  it("loads sessions, filters projects, and resumes the selected session", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "docs 1 session" }),
    );
    await screen.findByRole("heading", { name: "Document installation" });
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    expect(backend.ResumeChat).toHaveBeenCalledWith("claude/two");
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
  });
  it("starts a selected agent in the chosen project and prevents duplicate launches", async () => {
    let finish!: () => void;
    backend.NewChat = vi.fn(
      () =>
        new Promise<ChatState>((resolve) => {
          finish = () => resolve(live);
        }),
    );
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(screen.getByRole("button", { name: "New session" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.selectOptions(
      within(dialog).getByLabelText("Project"),
      "/demo/docs",
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: /Claude Code Ready/ }),
    );
    const start = within(dialog).getByRole("button", {
      name: "Start in app",
    });
    fireEvent.click(start);
    fireEvent.click(start);
    expect(backend.NewChat).toHaveBeenCalledTimes(1);
    expect(backend.NewChat).toHaveBeenCalledWith("/demo/docs", "claude");
    await act(async () => finish());
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
  it("switches between history and a live session without duplicating its tab", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "History" }));
    expect(
      screen.queryByRole("region", { name: "Chat conversation" }),
    ).not.toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    expect(
      screen.getAllByRole("button", { name: /Claude Code · docs/ }),
    ).toHaveLength(1);
    expect(
      screen.getByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
  });
  it("does not replace a newer preview with a late response", async () => {
    let first!: (value: Preview) => void;
    backend.Transcript = vi.fn((key: string): Promise<Preview> =>
      key === "codex/one"
        ? new Promise<Preview>((resolve) => {
            first = resolve;
          })
        : Promise.resolve({
            messages: [
              { role: "user", kind: "text", text: "The docs conversation" },
            ],
            truncated: false,
          }),
    );
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "docs 1 session" }),
    );
    await screen.findByText("The docs conversation");
    await act(async () =>
      first({
        messages: [{ role: "user", kind: "text", text: "Stale conversation" }],
        truncated: false,
      }),
    );
    expect(screen.queryByText("Stale conversation")).not.toBeInTheDocument();
  });
  it("shows launch failures without a false success notification", async () => {
    backend.ResumeChat = vi
      .fn()
      .mockRejectedValue(new Error("Agent could not start"));
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Agent could not start",
    );
    expect(screen.queryByText(/opened in app/)).not.toBeInTheDocument();
  });
  it("keeps empty-state actions useful and ignores a canceled folder picker", async () => {
    backend.Refresh = vi
      .fn()
      .mockResolvedValue({ ...snapshot, sessions: [], projects: [] });
    render(<App />);
    await screen.findByText("A fresh start");
    await userEvent.click(
      screen.getByRole("button", { name: "Choose a project" }),
    );
    expect(backend.PickProject).toHaveBeenCalledTimes(1);
    expect(backend.NewChat).not.toHaveBeenCalled();
  });
  it("does not load transcript images or turn transcript HTML into active content", async () => {
    backend.Transcript = vi.fn().mockResolvedValue({
      messages: [
        {
          role: "assistant",
          kind: "text",
          text: "<script>window.evil=true</script>\n\n![private](https://example.invalid/track)\n\n[link](javascript:alert(1))",
        },
      ],
      truncated: false,
    });
    render(<App />);
    await screen.findByText("[Image omitted from preview]");
    expect(document.querySelector(".messages img")).toBeNull();
    expect(document.querySelector(".messages script")).toBeNull();
    expect(document.querySelector(".messages a")).toBeNull();
  });
});
