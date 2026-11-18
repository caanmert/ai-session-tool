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
    focused = true,
    pane,
    onFocusPane,
    onClosePane,
  }: {
    chat: ChatState;
    active: boolean;
    focused?: boolean;
    pane?: string;
    onFocusPane?: () => void;
    onClosePane?: () => void;
  }) => (
    <section
      hidden={!active}
      aria-label="Chat conversation"
      data-pane={pane || ""}
      data-focused={String(focused)}
    >
      {session.id}
      {pane && !focused && (
        <button onClick={onFocusPane}>Focus {session.id}</button>
      )}
      {onClosePane && (
        <button onClick={onClosePane}>Close pane {session.id}</button>
      )}
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
    AgentHealth: vi.fn().mockResolvedValue([]),
    Notify: vi.fn().mockResolvedValue(undefined),
    RevealProject: vi.fn(),
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
    ChatEarlier: vi.fn(),
    CodexHolder: vi.fn().mockResolvedValue(""),
    SendChat: vi.fn(),
    SendChatWithImages: vi.fn(),
    StopChat: vi.fn(),
    CloseChat: vi.fn(),
    AnswerChat: vi.fn(),
    PrepareReview: vi.fn(),
    PickFiles: vi.fn(),
    CommitPlan: vi.fn(),
    Commit: vi.fn(),
    PullRequestPlan: vi.fn(),
    CreatePullRequest: vi.fn(),
    OpenURL: vi.fn(),
    StartComparison: vi.fn(),
    ComparisonOverview: vi.fn(),
    ComparisonDiff: vi.fn(),
    UseComparisonSide: vi.fn(),
    RemoveComparison: vi.fn(),
    UsageLimits: vi.fn().mockResolvedValue({ agents: [] }),
    GitHubIssueForm: vi.fn(),
    AgentSkills: vi.fn().mockResolvedValue({ tool: "codex", skills: [] }),
    SetChatWebSearch: vi.fn(),
    CreateGitHubIssue: vi.fn(),
    GitHubOverview: vi.fn(),
    GitHubIssue: vi.fn(),
    GitHubPull: vi.fn(),
    RestorePlan: vi.fn(),
    SearchMessages: vi.fn().mockResolvedValue([]),
    ProjectInstructions: vi.fn(),
    ProjectCommands: vi.fn().mockResolvedValue([]),
    HandOffChat: vi.fn(),
    ContinueChat: vi.fn(),
    CompactChat: vi.fn(),
    SavedPrompts: vi.fn().mockResolvedValue([]),
    NewChatInCopy: vi.fn(),
    CopyChanges: vi.fn(),
    BringBackCopy: vi.fn(),
    RemoveCopy: vi.fn(),
    SetSavedPrompts: vi.fn(),
    TurnDiff: vi.fn(),
    StartProjectCommand: vi.fn(),
    ProjectRun: vi
      .fn()
      .mockRejectedValue(new Error("nothing has run here yet")),
    StopProjectCommand: vi.fn(),
    ReadInstruction: vi.fn(),
    WriteInstruction: vi.fn(),
    RevealInstruction: vi.fn(),
    RestoreCheckpoint: vi.fn(),
    DiscardFile: vi.fn(),
    ChatModes: vi.fn().mockResolvedValue([]),
    SetChatMode: vi.fn(),
    AnswerChatOption: vi.fn(),
    ProjectFiles: vi.fn(),
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
      within(dialog).getByRole("radio", { name: /Claude Code Ready/ }),
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
  it("starts a task with its first message in one step", async () => {
    backend.SendChat = vi.fn().mockResolvedValue(undefined);
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    const dialog = await screen.findByRole("dialog");
    const task = within(dialog).getByLabelText("Task");
    expect(task).toHaveFocus();
    // Without a task the button only opens the conversation.
    expect(
      within(dialog).getByRole("button", { name: "Start in app" }),
    ).toBeVisible();
    await userEvent.type(task, "Add retries{Enter}and tests");
    // Enter adds a line; it does not start anything.
    expect(backend.NewChat).not.toHaveBeenCalled();
    expect(
      within(dialog).getByText(/starts working as soon as you start/),
    ).toBeVisible();
    await userEvent.click(
      within(dialog).getByRole("button", { name: /Start task/ }),
    );
    await waitFor(() =>
      expect(backend.SendChat).toHaveBeenCalledWith(
        "live-one",
        "Add retries\nand tests",
      ),
    );
    expect(backend.NewChat).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
  });
  it("starts a task in its own copy of the project on request", async () => {
    backend.SendChat = vi.fn().mockResolvedValue(undefined);
    backend.NewChatInCopy = vi.fn().mockResolvedValue({
      ...live,
      copy: {
        project: "/demo/docs",
        base: "abc123",
        branch: "ais/1a2b",
        worktree: "/data/copies/1a2b/docs",
      },
    });
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Task"), "Refactor");
    const copy = within(dialog).getByRole("checkbox", {
      name: /Work in a separate copy/,
    });
    expect(copy).not.toBeChecked();
    await userEvent.click(copy);
    await userEvent.click(
      within(dialog).getByRole("button", { name: /Start task/ }),
    );
    await waitFor(() =>
      expect(backend.SendChat).toHaveBeenCalledWith("live-one", "Refactor"),
    );
    expect(backend.NewChatInCopy).toHaveBeenCalledWith("/demo/api", "codex");
    expect(backend.NewChat).not.toHaveBeenCalled();
  });
  it("preselects the agent last started in a project", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    let dialog = await screen.findByRole("dialog");
    await userEvent.selectOptions(
      within(dialog).getByLabelText("Project"),
      "/demo/docs",
    );
    await userEvent.click(
      within(dialog).getByRole("radio", { name: /Claude Code Ready/ }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Start in app" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(backend.NewChat).toHaveBeenCalledWith("/demo/docs", "claude");
    // Next time, choosing that project selects Claude Code again; another
    // project keeps the usual default.
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    dialog = await screen.findByRole("dialog");
    await userEvent.selectOptions(
      within(dialog).getByLabelText("Project"),
      "/demo/api",
    );
    await userEvent.click(
      within(dialog).getByRole("radio", { name: /Codex Ready/ }),
    );
    await userEvent.selectOptions(
      within(dialog).getByLabelText("Project"),
      "/demo/docs",
    );
    expect(
      within(dialog).getByRole("radio", { name: /Claude Code Ready/ }),
    ).toHaveAttribute("aria-checked", "true");
  });
  it("chooses the agent for a new task with arrow keys, as a radio group", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    const dialog = await screen.findByRole("dialog");
    const group = within(dialog).getByRole("radiogroup", { name: "Agent" });
    const codex = within(group).getByRole("radio", { name: /Codex Ready/ });
    const claude = within(group).getByRole("radio", {
      name: /Claude Code Ready/,
    });
    const both = within(group).getByRole("radio", {
      name: "Both, compared Each in its own worktree",
    });
    expect(codex).toHaveAttribute("aria-checked", "true");
    // Only the chosen option is in the Tab order.
    expect(codex).toHaveAttribute("tabindex", "0");
    expect(claude).toHaveAttribute("tabindex", "-1");
    codex.focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(claude).toHaveAttribute("aria-checked", "true");
    expect(claude).toHaveFocus();
    await userEvent.keyboard("{ArrowRight}");
    expect(both).toHaveAttribute("aria-checked", "true");
    await userEvent.keyboard("{ArrowDown}");
    expect(codex).toHaveAttribute("aria-checked", "true");
    await userEvent.keyboard("{ArrowUp}");
    expect(both).toHaveAttribute("aria-checked", "true");
    expect(codex).toHaveAttribute("aria-checked", "false");
  });
  it("keeps the task as a draft when the first message cannot be sent", async () => {
    backend.SendChat = vi
      .fn()
      .mockRejectedValue(new Error("agent not signed in"));
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Task"), "Add retries");
    fireEvent.keyDown(within(dialog).getByLabelText("Task"), {
      key: "Enter",
      metaKey: true,
    });
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /conversation is open, but your message was not sent: agent not signed in/,
    );
    expect(window.localStorage.getItem("ais:chat-draft:v1:live-one")).toBe(
      "Add retries",
    );
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
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
    // Conversations are searched by their text too before nothing is said to match.
    expect(await screen.findByText(/Nothing matches/)).toBeVisible();
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
    // Notifications are on by default and can be turned off.
    const notify = within(settings).getByRole("checkbox", {
      name: /Notify me when an agent needs me or finishes/,
    });
    expect(notify).toBeChecked();
    await userEvent.click(notify);
    expect(notify).not.toBeChecked();
    expect(window.localStorage.getItem("ais:notifications:v1")).toBe("off");
    // The narrower choice depends on notifications being on.
    const needsOnly = within(settings).getByRole("checkbox", {
      name: /Only when an agent needs me/,
    });
    expect(needsOnly).toBeDisabled();
    await userEvent.click(notify);
    await userEvent.click(needsOnly);
    expect(window.localStorage.getItem("ais:notifications-needs:v1")).toBe(
      "on",
    );
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
  it("opens a conversation when its notification is clicked", async () => {
    let fire: (id: unknown) => void = () => {};
    window.runtime = {
      EventsOn: (name, callback) => {
        if (name === "notification:open") fire = callback;
        return () => {};
      },
    };
    try {
      render(<App />);
      await screen.findByRole("heading", { name: "Retry webhook delivery" });
      fire(live.id);
      await waitFor(() =>
        expect(backend.ReopenChat).toHaveBeenCalledWith(live.id),
      );
      expect(
        await screen.findByRole("region", { name: "Chat conversation" }),
      ).toBeVisible();
    } finally {
      window.runtime = undefined;
    }
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
  it("warns when an installed agent no longer accepts what ais sends, and explains it in Settings", async () => {
    backend.AgentHealth = vi.fn().mockResolvedValue([
      { tool: "codex", version: "0.167.1", tested: "0.162.1" },
      {
        tool: "claude",
        version: "3.0.0",
        tested: "2.1.296",
        problem: "This version rejects an option ais relies on: unknown option",
      },
    ]);
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    const warning = await screen.findByText(
      /Claude Code may not work with this version of ais/,
    );
    await userEvent.click(
      within(warning.closest("[role=alert]") as HTMLElement).getByRole(
        "button",
        { name: "See details in Settings" },
      ),
    );
    const settings = screen.getByRole("region", { name: "Settings" });
    expect(within(settings).getByText("Not compatible")).toBeVisible();
    expect(within(settings).getByText(/unknown option/)).toBeVisible();
    // A newer version that still works is shown without comment.
    expect(within(settings).getByText("Version 0.167.1")).toBeVisible();
    expect(within(settings).queryByText(/newer than/)).not.toBeInTheDocument();
    expect(
      within(settings).getByText(/does not contact a model/),
    ).toBeVisible();
    // Check again asks the agents again.
    await userEvent.click(
      within(settings).getByRole("button", { name: "Check again" }),
    );
    expect(backend.AgentHealth).toHaveBeenCalledTimes(2);
  });
  it("opens Settings with ⌘, and switches to an open conversation with ⌘1", async () => {
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: ",", metaKey: true });
    expect(screen.getByRole("region", { name: "Settings" })).toBeVisible();
    // Nothing is open yet, so ⌘1 leaves the page alone.
    fireEvent.keyDown(document, { key: "1", metaKey: true });
    expect(screen.getByRole("region", { name: "Settings" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /^History/ }));
    await userEvent.click(
      screen.getByRole("button", { name: "Continue in app" }),
    );
    await screen.findByRole("region", { name: "Chat conversation" });
    await userEvent.click(screen.getByRole("button", { name: "Settings" }));
    expect(
      screen.queryByRole("region", { name: "Chat conversation" }),
    ).not.toBeInTheDocument();
    fireEvent.keyDown(document, { key: "1", metaKey: true });
    expect(
      await screen.findByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
  });
  it("starts a task from a GitHub issue through New task, without sending anything", async () => {
    backend.GitHubOverview = vi.fn().mockResolvedValue({
      repository: "me/docs",
      url: "https://github.com/me/docs",
      issues: [
        {
          number: 4,
          title: "Explain installation",
          url: "https://github.com/me/docs/issues/4",
          author: "ana",
          labels: [],
          updatedAt: "2026-11-15T10:00:00Z",
        },
      ],
      pulls: [],
    });
    backend.GitHubIssue = vi.fn().mockResolvedValue({
      number: 4,
      title: "Explain installation",
      url: "https://github.com/me/docs/issues/4",
      body: "Cover Homebrew.",
    });
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Project" }),
      "/demo/docs",
    );
    await userEvent.click(screen.getByRole("button", { name: "Issues & PRs" }));
    const list = await screen.findByRole("list", { name: "Open issues" });
    expect(backend.GitHubOverview).toHaveBeenLastCalledWith("/demo/docs");
    await userEvent.click(
      within(list).getByRole("button", { name: "Start task" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("Task")).toHaveValue(
      "Work on issue #4: Explain installation\n\nhttps://github.com/me/docs/issues/4\n\nCover Homebrew.",
    );
    expect(within(dialog).getByLabelText("Project")).toHaveValue("/demo/docs");
    expect(backend.NewChat).not.toHaveBeenCalled();
    expect(backend.SendChat).not.toHaveBeenCalled();
  });
  it("starts a comparison of both agents from New task", async () => {
    const side = (id: string, tool: "claude" | "codex") => ({
      ...live,
      id,
      tool,
      title: "Add a greeting",
      comparison: "cmp-1",
      project: `/data/worktrees/cmp-1/docs-${tool}`,
    });
    backend.StartComparison = vi.fn().mockResolvedValue({
      comparison: { id: "cmp-1", sides: [] },
      chats: [side("claude-side", "claude"), side("codex-side", "codex")],
      problem: "Codex could not start: eight agents are already running.",
    });
    backend.ReadChat = vi.fn(async (id: string, version: number) => ({
      version: 1,
      state:
        version === 1
          ? null
          : side(id, id === "claude-side" ? "claude" : "codex"),
    }));
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    fireEvent.keyDown(document, { key: "n", metaKey: true });
    const dialog = await screen.findByRole("dialog");
    await userEvent.selectOptions(
      within(dialog).getByLabelText("Project"),
      "/demo/docs",
    );
    await userEvent.click(
      within(dialog).getByRole("radio", { name: /Both, compared/ }),
    );
    // A comparison needs the task text.
    expect(
      within(dialog).getByRole("button", { name: /Start comparison/ }),
    ).toBeDisabled();
    await userEvent.type(
      within(dialog).getByLabelText("Task"),
      "Add a greeting",
    );
    expect(
      within(dialog).getByText(/each in its own Git worktree/),
    ).toBeVisible();
    await userEvent.click(
      within(dialog).getByRole("button", { name: /Start comparison/ }),
    );
    expect(backend.StartComparison).toHaveBeenCalledWith(
      "/demo/docs",
      "Add a greeting",
    );
    expect(backend.NewChat).not.toHaveBeenCalled();
    // Both conversations open; a side that could not start is explained.
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "eight agents are already running",
    );
    const sidebar = screen.getByRole("navigation", {
      name: "Open conversations",
    });
    expect(within(sidebar).getAllByText("Add a greeting")).toHaveLength(2);
    expect(
      screen.getByRole("region", { name: "Chat conversation" }),
    ).toBeVisible();
  });
  describe("two conversations side by side", () => {
    const chat = (id: string, title: string): ChatState => ({
      ...live,
      id,
      title,
      sourceKey: "",
    });
    const panes = () =>
      screen
        .getAllByRole("region", { name: "Chat conversation", hidden: true })
        .filter((section) => !section.hidden)
        .map(
          (section) =>
            `${section.firstChild?.textContent}:${section.dataset.pane || "-"}:${section.dataset.focused}`,
        );
    beforeEach(() => {
      window.localStorage.clear();
      backend.ListChats = vi
        .fn()
        .mockResolvedValue([
          chat("a", "First task"),
          chat("b", "Second task"),
          chat("c", "Third task"),
        ]);
    });
    it("opens beside, moves focus, replaces the focused pane and closes a pane", async () => {
      render(<App />);
      const sidebar = await screen.findByRole("navigation", {
        name: "Open conversations",
      });
      await userEvent.click(
        await within(sidebar).findByRole("button", { name: /^First task/ }),
      );
      expect(panes()).toEqual(["a:-:true"]);
      await userEvent.click(
        within(sidebar).getByRole("button", {
          name: "Open Second task beside",
        }),
      );
      expect(panes()).toEqual(["a:first:false", "b:second:true"]);
      // Neither shown conversation offers "beside" again; the third does.
      expect(
        within(sidebar).queryByRole("button", {
          name: "Open First task beside",
        }),
      ).toBeNull();
      // Clicking into a pane focuses it.
      await userEvent.click(screen.getByRole("button", { name: "Focus a" }));
      expect(panes()).toEqual(["a:first:true", "b:second:false"]);
      // Choosing another conversation replaces the focused pane.
      await userEvent.click(
        within(sidebar).getByRole("button", { name: /^Third task/ }),
      );
      expect(panes()).toEqual(["b:second:false", "c:first:true"]);
      // Closing a pane leaves the other conversation, still open in the sidebar.
      await userEvent.click(
        screen.getByRole("button", { name: "Close pane c" }),
      );
      expect(panes()).toEqual(["b:-:true"]);
      expect(
        within(sidebar).getByRole("button", { name: /^Third task/ }),
      ).toBeVisible();
    });
    it("offers the split in the palette and leaves the other side when one closes", async () => {
      render(<App />);
      const sidebar = await screen.findByRole("navigation", {
        name: "Open conversations",
      });
      await userEvent.click(
        await within(sidebar).findByRole("button", { name: /^First task/ }),
      );
      await userEvent.keyboard("{Meta>}k{/Meta}");
      const palette = await screen.findByRole("dialog", {
        name: "Search and commands",
      });
      await userEvent.type(
        within(palette).getByRole("combobox"),
        "beside second",
      );
      await userEvent.click(
        within(palette).getByRole("option", {
          name: /Open beside: Second task/,
        }),
      );
      expect(panes()).toEqual(["a:first:false", "b:second:true"]);
      backend.CloseChat = vi.fn().mockResolvedValue(undefined);
      await userEvent.click(
        within(sidebar).getByRole("button", { name: "Close Second task" }),
      );
      await waitFor(() => expect(panes()).toEqual(["a:-:true"]));
    });
    it("shows only the focused pane on a narrow window", async () => {
      window.localStorage.setItem(
        "ais:split:v1",
        JSON.stringify({ first: "a", second: "b" }),
      );
      const original = window.matchMedia;
      window.matchMedia = ((query: string) => ({
        matches: query.includes("max-width"),
        addEventListener: () => {},
        removeEventListener: () => {},
      })) as unknown as typeof window.matchMedia;
      try {
        render(<App />);
        const sidebar = await screen.findByRole("navigation", {
          name: "Open conversations",
        });
        await userEvent.click(
          await within(sidebar).findByRole("button", { name: /^Second task/ }),
        );
        expect(panes()).toEqual(["b:-:true"]);
        expect(
          within(sidebar).queryByRole("button", { name: /beside$/ }),
        ).toBeNull();
      } finally {
        window.matchMedia = original;
      }
    });
  });
  it("shows usage limits from the sidebar and reads them again on request", async () => {
    backend.UsageLimits = vi.fn().mockResolvedValue({
      agents: [
        {
          tool: "claude",
          plan: "Pro",
          limits: [
            {
              label: "5-hour session",
              window: "session",
              percent: 40,
              resetsAt: new Date(Date.now() + 3_600_000).toISOString(),
              level: "normal",
            },
          ],
          readAt: new Date().toISOString(),
        },
      ],
    });
    render(<App />);
    await screen.findByRole("heading", { name: "Retry webhook delivery" });
    expect(backend.UsageLimits).toHaveBeenCalledWith(false);
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Claude Code: Session 40%. Open usage limits",
      }),
    );
    expect(
      screen.getByRole("heading", { name: "Usage limits", level: 1 }),
    ).toBeVisible();
    expect(
      screen.getByRole("meter", { name: "Claude Code: 5-hour session" }),
    ).toHaveAttribute("aria-valuenow", "40");
    await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(backend.UsageLimits).toHaveBeenLastCalledWith(true);
  });
  it("narrows the sidebar with Cmd+B and remembers the choice", async () => {
    try {
      render(<App />);
      await screen.findByRole("heading", { name: "Retry webhook delivery" });
      const sidebar = () =>
        screen.getByRole("complementary", { name: "Workspace" });
      expect(
        within(sidebar()).getByRole("button", { name: /New task/ }),
      ).toHaveTextContent("New task");
      fireEvent.keyDown(document, { key: "b", metaKey: true });
      const newTask = within(sidebar()).getByRole("button", {
        name: "New task",
      });
      expect(newTask).not.toHaveTextContent("New task");
      expect(window.localStorage.getItem("ais:sidebar-rail:v1")).toBe("true");
      // Every page is still one click away.
      await userEvent.click(
        within(sidebar()).getByRole("button", { name: "Settings" }),
      );
      expect(
        screen.getByRole("heading", { name: "Settings", level: 1 }),
      ).toBeVisible();
      await userEvent.click(
        screen.getByRole("button", { name: "Expand sidebar" }),
      );
      expect(
        within(sidebar()).getByRole("button", { name: /New task/ }),
      ).toHaveTextContent("New task");
      expect(window.localStorage.getItem("ais:sidebar-rail:v1")).toBe("false");
    } finally {
      window.localStorage.removeItem("ais:sidebar-rail:v1");
    }
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
