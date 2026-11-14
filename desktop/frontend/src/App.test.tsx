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
    SetAppearance: vi.fn().mockResolvedValue(undefined),
    SetAttention: vi.fn().mockResolvedValue(undefined),
    AnnotateSession: vi.fn(),
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
    ListSavedChats: vi.fn().mockResolvedValue([]),
    ReopenChat: vi.fn().mockResolvedValue(live),
    NewSession: vi.fn(),
    ResumeSession: vi.fn(),
    LiveSessions: vi.fn(),
    ReadChat: vi.fn(),
    SendChat: vi.fn(),
    StopChat: vi.fn(),
    CloseChat: vi.fn(),
    AnswerChat: vi.fn(),
    PrepareReview: vi.fn(),
    PickFiles: vi.fn(),
    ChatChanges: vi
      .fn()
      .mockResolvedValue({ available: true, files: [], added: 0, removed: 0 }),
    ChatDiff: vi.fn(),
    StartReview: vi.fn(),
    SendReviewFindings: vi.fn(),
    ReadTerminal: vi.fn(),
    WriteTerminal: vi.fn(),
    ResizeTerminal: vi.fn(),
    StopTerminal: vi.fn(),
    CloseTerminal: vi.fn(),
    ListRuns: vi.fn().mockResolvedValue([]),
    RunStatus: vi.fn(),
    RunDiff: vi.fn(),
    StartRun: vi.fn(),
    StopRun: vi.fn(),
    RunAttachCommand: vi.fn(),
  };
  window.go = { main: { App: backend } };
});
describe("desktop workflows", () => {
  it("combines status, agent and search filters and clears an empty result", async () => {
    backend.Refresh = vi.fn().mockResolvedValue({
      ...snapshot,
      sessions: snapshot.sessions.map((s, i) => ({
        ...s,
        pinned: i === 1,
        live: i === 0 ? { pid: 123 } : undefined,
      })),
    });
    render(<App />);
    await screen.findByRole("heading", { name: "Document installation" });
    await userEvent.click(screen.getByRole("button", { name: "Pinned 1" }));
    const list = screen.getByRole("region", { name: "Conversations" });
    expect(within(list).getAllByRole("button")).toHaveLength(1);
    await userEvent.click(
      within(screen.getByLabelText("Filter by agent")).getByRole("button", {
        name: "Codex",
      }),
    );
    await screen.findByText("No pinned conversations");
    await userEvent.click(
      screen.getByRole("button", { name: "Show all conversations" }),
    );
    expect(within(list).getAllByRole("button")).toHaveLength(2);
    await userEvent.click(screen.getByRole("button", { name: "Live 1" }));
    expect(within(list).getAllByRole("button")).toHaveLength(1);
    await userEvent.type(
      screen.getByRole("textbox", { name: "Search sessions" }),
      "webhook",
    );
    await waitFor(() => expect(backend.Search).toHaveBeenCalledWith("webhook"));
    await userEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(
      screen.getByRole("textbox", { name: "Search sessions" }),
    ).toHaveFocus();
    expect(
      screen.getByRole("textbox", { name: "Search sessions" }),
    ).toHaveValue("");
    expect(screen.getByRole("button", { name: "Live 1" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
  it("sorts without changing the source snapshot and navigates with the keyboard", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    const list = screen.getByRole("region", { name: "Conversations" });
    const search = screen.getByRole("textbox", { name: "Search sessions" });
    search.focus();
    fireEvent.keyDown(search, { key: "ArrowDown" });
    expect(within(list).getAllByRole("button")[0]).toHaveFocus();
    await userEvent.keyboard("{End}");
    expect(within(list).getAllByRole("button")[1]).toHaveFocus();
    expect(
      screen.getByRole("heading", { name: "Document installation" }),
    ).toBeVisible();
    await userEvent.keyboard("{Home}{ArrowDown}{ArrowUp}");
    expect(within(list).getAllByRole("button")[0]).toHaveFocus();
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Sort conversations" }),
      "oldest",
    );
    expect(within(list).getAllByRole("button")[0]).toHaveTextContent(
      "Document installation",
    );
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Sort conversations" }),
      "title",
    );
    expect(within(list).getAllByRole("button")[0]).toHaveTextContent(
      "Document installation",
    );
    expect(snapshot.sessions[0].key).toBe("codex/one");
    fireEvent.keyDown(search, { key: "Enter" });
    expect(backend.ResumeChat).not.toHaveBeenCalled();
    fireEvent.keyDown(search, { key: "ArrowDown" });
    await userEvent.keyboard("{Home}{Enter}");
    expect(backend.ResumeChat).toHaveBeenCalledWith("claude/two");
  });
  it("loads sessions, filters projects, and resumes the selected session", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Project" }),
      "/demo/docs",
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
    await userEvent.click(screen.getByRole("button", { name: /New task/ }));
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
  it("switches between history and a live session without duplicating its sidebar row", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /^History/ }));
    expect(
      screen.queryByRole("region", { name: "Chat conversation" }),
    ).not.toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    expect(
      screen.getAllByRole("button", { name: /Claude · Ready/ }),
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
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Project" }),
      "/demo/docs",
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
  it("opens the command palette with ⌘K and jumps to a saved conversation without starting an agent", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "Parallel runs" }),
    );
    await screen.findByText("No runs yet");
    fireEvent.keyDown(document, { key: "k", metaKey: true });
    const palette = await screen.findByRole("dialog", {
      name: "Search and commands",
    });
    const input = within(palette).getByRole("combobox");
    expect(
      within(palette).getByRole("option", { name: /New task/ }),
    ).toHaveAttribute("aria-selected", "true");
    await userEvent.type(input, "document claude");
    const options = within(palette).getAllByRole("option");
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent("Document installation");
    await userEvent.keyboard("{Enter}");
    expect(
      screen.queryByRole("dialog", { name: "Search and commands" }),
    ).not.toBeInTheDocument();
    await screen.findByRole("heading", { name: "Document installation" });
    expect(backend.ResumeChat).not.toHaveBeenCalled();
    expect(backend.NewChat).not.toHaveBeenCalled();
  });
  it("moves through palette results with the arrow keys and reports no matches", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: /Search conversations, projects/ }),
    );
    const palette = await screen.findByRole("dialog", {
      name: "Search and commands",
    });
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expect(
      within(palette).getByRole("option", { name: /Parallel runs/ }),
    ).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{Enter}");
    await screen.findByText("No runs yet");
    fireEvent.keyDown(document, { key: "k", metaKey: true });
    await userEvent.type(
      within(
        await screen.findByRole("dialog", { name: "Search and commands" }),
      ).getByRole("combobox"),
      "zzzz",
    );
    expect(screen.getByRole("status")).toHaveTextContent("Nothing matches");
  });
  it("explains what to do when no agent is installed", async () => {
    backend.Refresh = vi.fn().mockResolvedValue({
      ...snapshot,
      sessions: [],
      tools: snapshot.tools.map((tool) => ({ ...tool, available: false })),
    });
    render(<App />);
    const setup = await screen.findByRole("region", { name: "Agent setup" });
    expect(within(setup).getAllByText("Not found")).toHaveLength(2);
    expect(
      within(setup).getByText("npm install -g @anthropic-ai/claude-code"),
    ).toBeVisible();
    expect(within(setup).getByText(/Nothing is sent/)).toBeVisible();
    await userEvent.click(
      within(setup).getByRole("button", { name: "Check again" }),
    );
    expect(backend.Refresh).toHaveBeenCalledTimes(2);
  });
  it("keeps appearance, agent status and shortcuts in Settings", async () => {
    backend.Refresh = vi.fn().mockResolvedValue({
      ...snapshot,
      tools: [
        { name: "codex", path: "/bin/codex", available: true },
        { name: "claude", path: "", available: false },
      ],
    });
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(screen.getByRole("button", { name: "Settings" }));
    const settings = screen.getByRole("region", { name: "Settings" });
    expect(screen.getByRole("button", { name: "Settings" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await userEvent.selectOptions(
      within(settings).getByLabelText("Appearance"),
      "dark",
    );
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(within(settings).getByText("/bin/codex")).toBeVisible();
    expect(within(settings).getByText("Found")).toBeVisible();
    expect(within(settings).getByText("Not found")).toBeVisible();
    expect(within(settings).getByText("⌘K")).toBeVisible();
    await userEvent.click(
      within(settings).getByRole("button", { name: "Check again" }),
    );
    expect(backend.Refresh).toHaveBeenCalledTimes(2);
    // The theme stays applied after leaving Settings.
    await userEvent.click(screen.getByRole("button", { name: /^History/ }));
    expect(
      screen.queryByRole("region", { name: "Settings" }),
    ).not.toBeInTheDocument();
    expect(document.documentElement.dataset.theme).toBe("dark");
  });
  it("renames, pins and tags a saved conversation without starting an agent", async () => {
    const annotated = (change: Partial<(typeof snapshot.sessions)[0]>) => ({
      ...snapshot,
      sessions: snapshot.sessions.map((s, i) =>
        i === 0 ? { ...s, ...change } : s,
      ),
    });
    backend.AnnotateSession = vi
      .fn()
      .mockResolvedValueOnce(annotated({ title: "Webhook retries" }))
      .mockResolvedValueOnce(
        annotated({ title: "Webhook retries", pinned: true }),
      )
      .mockResolvedValueOnce(
        annotated({
          title: "Webhook retries",
          pinned: true,
          tags: ["backend"],
        }),
      )
      .mockRejectedValueOnce(new Error("invalid tag"));
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "Rename conversation" }),
    );
    const title = screen.getByRole("textbox", { name: "Conversation title" });
    expect(title).toHaveValue("Retry webhook delivery");
    await userEvent.clear(title);
    await userEvent.type(title, "  Webhook retries {Enter}");
    expect(backend.AnnotateSession).toHaveBeenLastCalledWith("codex/one", {
      title: "Webhook retries",
    });
    await screen.findByRole("heading", { name: "Webhook retries" });

    const pin = screen.getByRole("button", { name: "Pin conversation" });
    expect(pin).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(pin);
    expect(backend.AnnotateSession).toHaveBeenLastCalledWith("codex/one", {
      pinned: true,
    });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Pin conversation" }),
      ).toHaveAttribute("aria-pressed", "true"),
    );

    await userEvent.type(
      screen.getByRole("textbox", { name: "Add a tag" }),
      "backend{Enter}",
    );
    expect(backend.AnnotateSession).toHaveBeenLastCalledWith("codex/one", {
      addTags: ["backend"],
    });
    expect(
      await screen.findByRole("button", { name: "Remove tag backend" }),
    ).toBeVisible();

    // A rejected change is reported and nothing else changes.
    await userEvent.type(
      screen.getByRole("textbox", { name: "Add a tag" }),
      "not a tag!{Enter}",
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("invalid tag");
    expect(backend.ResumeChat).not.toHaveBeenCalled();
  });
  it("opens saved desktop work without starting or resuming a provider", async () => {
    backend.ListSavedChats = vi.fn().mockResolvedValue([
      {
        id: live.id,
        tool: live.tool,
        project: live.project,
        title: "Saved checkout review",
        status: "idle",
        updatedAt: live.updatedAt,
        closed: true,
        isReview: true,
        complete: true,
        findingCount: 1,
        sentCount: 0,
        fileCount: 2,
      },
    ]);
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.click(
      screen.getByRole("button", { name: "Saved workspace" }),
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Reopen Saved checkout review",
      }),
    );
    expect(backend.ReopenChat).toHaveBeenCalledWith(live.id);
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(backend.ResumeChat).not.toHaveBeenCalled();
    expect(backend.SendChat).not.toHaveBeenCalled();
  });
});
