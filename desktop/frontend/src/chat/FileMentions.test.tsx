import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import ChatWorkspace from "../ChatWorkspace";
import type { Backend, ChatState } from "../api";
import { insertMention, mentionAt } from "./FileMentions";

test("finds the mention being typed at the caret", () => {
  expect(mentionAt("@che", 4)).toEqual({ start: 0, query: "che" });
  expect(mentionAt("Compare @src/ch with", 15)).toEqual({
    start: 8,
    query: "src/ch",
  });
  expect(mentionAt("Compare @", 9)).toEqual({ start: 8, query: "" });
  // An address or a finished mention is not a mention being typed.
  expect(mentionAt("mail me@example", 15)).toBeNull();
  expect(mentionAt("@done and more", 14)).toBeNull();
  expect(mentionAt("no mention here", 5)).toBeNull();
});

test("inserts a path in place of the mention, quoting spaces", () => {
  expect(
    insertMention("Compare @che with tests", 12, 8, "src/checkout.ts"),
  ).toEqual({ text: "Compare src/checkout.ts with tests", caret: 24 });
  expect(insertMention("@no", 3, 0, "docs/release notes.md").text).toBe(
    '"docs/release notes.md" ',
  );
});

const chat = {
  id: "chat",
  tool: "codex",
  project: "/project",
  title: "Fix checkout",
  status: "idle",
  error: "",
  messages: [
    { id: "u1", role: "user", kind: "text", text: "First request" },
    { id: "a1", role: "assistant", kind: "text", text: "Done." },
    { id: "u2", role: "user", kind: "text", text: "Second request" },
  ],
  approvals: [],
  shortened: false,
  version: 1,
} as unknown as ChatState;
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  window.localStorage.clear();
  backend = {
    ReadChat: vi.fn(async (_id: string, version: number) => ({
      version: 1,
      state: version === 1 ? null : chat,
    })),
    ChatModels: vi
      .fn()
      .mockResolvedValue({ models: [], defaultModel: "", defaultEffort: "" }),
    ChatChanges: vi
      .fn()
      .mockResolvedValue({ available: true, files: [], added: 0, removed: 0 }),
    ProjectFiles: vi
      .fn()
      .mockResolvedValue(["src/checkout.ts", "src/checkout.test.ts"]),
    SendChat: vi.fn().mockResolvedValue(undefined),
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

test("typing @ lists project files and Enter inserts the chosen path without sending", async () => {
  show();
  const message = screen.getByLabelText("Message agent");
  await userEvent.type(message, "Compare @che");
  const list = await screen.findByRole("listbox", {
    name: "Files in this project",
  });
  expect(backend.ProjectFiles).toHaveBeenLastCalledWith("chat", "che");
  expect(within(list).getAllByRole("option")).toHaveLength(2);
  await userEvent.keyboard("{ArrowDown}{Enter}");
  expect(message).toHaveValue("Compare src/checkout.test.ts ");
  expect(list).not.toBeInTheDocument();
  expect(backend.SendChat).not.toHaveBeenCalled();
  // With the list closed, Enter sends as usual.
  await userEvent.keyboard("{Enter}");
  expect(backend.SendChat).toHaveBeenCalledWith(
    "chat",
    "Compare src/checkout.test.ts ",
  );
});

test("Escape closes the file list and keeps the text", async () => {
  show();
  const message = screen.getByLabelText("Message agent");
  await userEvent.type(message, "@che");
  await screen.findByRole("listbox");
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  expect(message).toHaveValue("@che");
});

test("says so when no file matches", async () => {
  backend.ProjectFiles.mockResolvedValue([]);
  show();
  await userEvent.type(screen.getByLabelText("Message agent"), "@zzz");
  expect(await screen.findByText(/No file matches/)).toBeVisible();
});

test("Up and Down in an empty message box step through earlier messages", async () => {
  show();
  const message = screen.getByLabelText("Message agent");
  await screen.findByText("Second request");
  fireEvent.keyDown(message, { key: "ArrowUp" });
  await waitFor(() => expect(message).toHaveValue("Second request"));
  fireEvent.keyDown(message, { key: "ArrowUp" });
  await waitFor(() => expect(message).toHaveValue("First request"));
  fireEvent.keyDown(message, { key: "ArrowDown" });
  await waitFor(() => expect(message).toHaveValue("Second request"));
  fireEvent.keyDown(message, { key: "ArrowDown" });
  await waitFor(() => expect(message).toHaveValue(""));
  // Text you typed yourself is never replaced.
  await userEvent.type(message, "my own text");
  fireEvent.keyDown(message, { key: "ArrowUp" });
  expect(message).toHaveValue("my own text");
  expect(backend.SendChat).not.toHaveBeenCalled();
});
