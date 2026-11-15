import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import ChatWorkspace from "../ChatWorkspace";
import type { Backend, ChatState } from "../api";
import { requestSummary } from "./conversation";

let chat: ChatState;
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  window.localStorage.clear();
  chat = {
    id: "chat",
    tool: "claude",
    project: "/project",
    title: "Rotate keys",
    status: "idle",
    error: "",
    mode: "",
    messages: [],
    approvals: [],
    shortened: false,
    version: 1,
  } as unknown as ChatState;
  backend = {
    ReadChat: vi.fn(async (_id: string, version: number) => ({
      version: chat.version,
      state: version === chat.version ? null : structuredClone(chat),
    })),
    ChatModels: vi
      .fn()
      .mockResolvedValue({ models: [], defaultModel: "", defaultEffort: "" }),
    ChatChanges: vi
      .fn()
      .mockResolvedValue({ available: true, files: [], added: 0, removed: 0 }),
    ChatModes: vi.fn().mockResolvedValue([
      { id: "plan", label: "Plan", detail: "Changes nothing." },
      { id: "", label: "Ask", detail: "Asks before editing." },
      {
        id: "acceptEdits",
        label: "Auto-edit",
        detail: "Edits without asking.",
      },
    ]),
    SetChatMode: vi.fn().mockResolvedValue(undefined),
    AnswerChat: vi.fn().mockResolvedValue(undefined),
    AnswerChatOption: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});
const show = () =>
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

test("shows a command request as the command, not as JSON", () => {
  expect(
    requestSummary(
      JSON.stringify({
        command: "npm test auth",
        description: "Run the tests",
      }),
    ),
  ).toEqual({ text: "npm test auth", note: "Run the tests" });
  // Anything that is not a command is shown as it was sent.
  const edit = JSON.stringify({ file_path: "a.ts", content: "x" });
  expect(requestSummary(edit)).toEqual({ text: edit, note: "" });
  expect(requestSummary("plain text")).toEqual({
    text: "plain text",
    note: "",
  });
});

test("offers the agent's wider grant next to Allow once and sends that choice", async () => {
  chat.status = "waiting";
  chat.approvals = [
    {
      id: "req",
      title: "Allow Bash?",
      detail: JSON.stringify({ command: "npm test", description: "Run tests" }),
      questions: [],
      options: [
        {
          id: "claude-rules",
          label: "Always allow in this project",
          detail: "Claude Code will allow: Bash(npm test)",
        },
      ],
    },
  ];
  show();
  const card = await screen.findByRole("region", { name: "Allow Bash?" });
  expect(within(card).getByLabelText("Action details")).toHaveTextContent(
    /^npm test$/,
  );
  expect(within(card).getByText("Run tests")).toBeVisible();
  // What the wider choice does is spelled out on the card.
  expect(
    within(card).getByText("Claude Code will allow: Bash(npm test)"),
  ).toBeVisible();
  expect(
    within(card).getByRole("button", { name: "Allow once" }),
  ).toBeVisible();
  await userEvent.click(
    within(card).getByRole("button", { name: "Always allow in this project" }),
  );
  expect(backend.AnswerChatOption).toHaveBeenCalledWith(
    "chat",
    "req",
    "claude-rules",
  );
  expect(backend.AnswerChat).not.toHaveBeenCalled();
});

test("a request without offered options has only Allow once and Deny", async () => {
  chat.status = "waiting";
  chat.approvals = [
    { id: "req", title: "Allow Bash?", detail: "npm test", questions: [] },
  ];
  show();
  const card = await screen.findByRole("region", { name: "Allow Bash?" });
  expect(within(card).getAllByRole("button")).toHaveLength(2);
});

test("presents a plan for approval as readable text", async () => {
  chat.status = "waiting";
  chat.mode = "plan";
  chat.approvals = [
    {
      id: "plan",
      title: "Approve this plan?",
      detail: "{}",
      questions: [],
      plan: "## Plan\n\n1. Add a key store\n2. Rotate daily",
    },
  ];
  show();
  const card = await screen.findByRole("region", {
    name: "Approve this plan?",
  });
  expect(within(card).getByRole("heading", { name: "Plan" })).toBeVisible();
  expect(within(card).getByText("Rotate daily")).toBeVisible();
  await userEvent.click(
    within(card).getByRole("button", { name: /Keep planning/ }),
  );
  expect(backend.AnswerChat).toHaveBeenCalledWith("chat", "plan", false, {});
  expect(
    within(card).getByRole("button", { name: /Approve plan|Sending/ }),
  ).toBeInTheDocument();
});

test("changes the permission mode from the message box", async () => {
  show();
  const picker = await screen.findByRole("combobox", {
    name: "Permission mode",
  });
  expect(picker).toHaveValue("");
  expect(
    within(picker)
      .getAllByRole("option")
      .map((o) => o.textContent),
  ).toEqual(["Plan", "Ask", "Auto-edit"]);
  await userEvent.selectOptions(picker, "acceptEdits");
  expect(backend.SetChatMode).toHaveBeenCalledWith("chat", "acceptEdits");
  // A refused change is reported next to the picker.
  backend.SetChatMode.mockRejectedValueOnce(
    new Error("finish the response first"),
  );
  await userEvent.selectOptions(picker, "plan");
  expect(await screen.findByText("finish the response first")).toBeVisible();
});

test("hides the mode picker when the agent lists no modes, and disables it while working", async () => {
  backend.ChatModes.mockResolvedValue([]);
  const view = show();
  await waitFor(() => expect(backend.ChatModes).toHaveBeenCalled());
  expect(
    screen.queryByRole("combobox", { name: "Permission mode" }),
  ).not.toBeInTheDocument();
  view.unmount();
  backend.ChatModes.mockResolvedValue([
    { id: "plan", label: "Plan", detail: "" },
    { id: "", label: "Default", detail: "" },
  ]);
  chat.status = "running";
  show();
  expect(
    await screen.findByRole("combobox", { name: "Permission mode" }),
  ).toBeDisabled();
});
