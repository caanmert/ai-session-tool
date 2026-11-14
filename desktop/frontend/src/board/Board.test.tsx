import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, ChatState, Tool } from "../api";
import Board from "./Board";

const chat = (id: string, patch: Partial<ChatState>): ChatState =>
  ({
    id,
    tool: "codex",
    project: "/work/checkout",
    title: `Task ${id}`,
    status: "idle",
    error: "",
    messages: [],
    approvals: [],
    ...patch,
  }) as ChatState;
const tools: Tool[] = [
  { name: "codex", path: "/bin/codex", available: true },
  { name: "claude", path: "", available: false },
];
const answer = vi.fn();
beforeEach(() => {
  answer.mockReset().mockResolvedValue(undefined);
  window.go = { main: { App: { AnswerChat: answer } as unknown as Backend } };
});

test("groups conversations by what they need, with Needs you first", () => {
  render(
    <Board
      tools={tools}
      onOpen={vi.fn()}
      chats={[
        chat("a", { status: "running", activeModel: "codex-demo" }),
        chat("b", { status: "error", error: "The agent exited (1)." }),
        chat("c", { tool: "claude", status: "stopped" }),
      ]}
    />,
  );
  const headings = screen
    .getAllByRole("heading", { level: 2 })
    .map((h) => h.textContent);
  expect(headings).toEqual(["Needs you0", "Working1", "Ready2"]);
  // Empty columns stay in place so the layout does not jump.
  expect(screen.getByText("Nothing is waiting for you.")).toBeVisible();
  expect(screen.getByText("The agent exited (1).")).toBeVisible();
  expect(screen.getByText(/Stopped by you/)).toBeVisible();
  expect(screen.getByText("1 working · 0 waiting · 2 open")).toBeVisible();
  expect(screen.getByText("Not found")).toBeVisible();
});

test("approves or denies a simple request in place and opens the conversation on request", async () => {
  const open = vi.fn();
  render(
    <Board
      tools={tools}
      onOpen={open}
      chats={[
        chat("a", {
          status: "waiting",
          approvals: [
            {
              id: "req-1",
              title: "Run checkout tests",
              detail: "npm test -- checkout",
              questions: [],
            },
          ],
        }),
      ]}
    />,
  );
  const card = screen.getByRole("article", { name: "Task a, Codex" });
  expect(within(card).getByText("Needs approval")).toBeVisible();
  expect(within(card).getByText("npm test -- checkout")).toBeVisible();
  await userEvent.click(within(card).getByRole("button", { name: "Deny" }));
  expect(answer).toHaveBeenCalledWith("a", "req-1", false, {});
  await userEvent.click(within(card).getByRole("button", { name: "Task a" }));
  expect(open).toHaveBeenCalledWith("a");
});

test("sends questions to the conversation and reports a failed answer", async () => {
  const open = vi.fn();
  answer.mockRejectedValue(new Error("This request was already answered."));
  render(
    <Board
      tools={tools}
      onOpen={open}
      chats={[
        chat("q", {
          status: "waiting",
          approvals: [
            {
              id: "req-q",
              title: "Choose a target",
              detail: "",
              questions: [{ id: "t", text: "Which target?", options: [] }],
            },
          ],
        }),
        chat("a", {
          status: "waiting",
          approvals: [
            {
              id: "req-a",
              title: "Delete dist",
              detail: "rm -rf dist",
              questions: [],
            },
          ],
        }),
      ]}
    />,
  );
  const question = screen.getByRole("article", { name: "Task q, Codex" });
  expect(within(question).getByText("Has a question")).toBeVisible();
  expect(
    within(question).queryByRole("button", { name: "Allow once" }),
  ).not.toBeInTheDocument();
  await userEvent.click(
    within(question).getByRole("button", { name: "Reply in conversation" }),
  );
  expect(open).toHaveBeenCalledWith("q");
  const approval = screen.getByRole("article", { name: "Task a, Codex" });
  await userEvent.click(
    within(approval).getByRole("button", { name: "Allow once" }),
  );
  expect(await within(approval).findByRole("alert")).toHaveTextContent(
    "already answered",
  );
});
