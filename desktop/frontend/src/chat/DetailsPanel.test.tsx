import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import type { ChatState, Session } from "../api";
import DetailsPanel from "./DetailsPanel";

const chat = {
  id: "chat",
  tool: "claude",
  project: "/work/checkout",
  activeModel: "claude-sonnet-5-5",
  title: "Rotate keys",
  messages: [],
} as unknown as ChatState;
const session = {
  userTurns: 4,
  assistantTurns: 9,
  usage: { input: 1200, output: 300, cacheRead: 46000, cacheWrite: 700 },
} as Session;
const summary = { available: true, files: [], added: 0, removed: 0 };
const panel = (saved?: Session) => (
  <DetailsPanel
    chat={chat}
    session={saved}
    summary={summary}
    error=""
    drawer={false}
    onRefresh={vi.fn()}
    onClose={vi.fn()}
  />
);

test("shows turns, tokens and the API-equivalent cost when they are known", () => {
  render(panel({ ...session, cost: 0.842 }));
  expect(screen.getByText("4 prompts · 9 replies")).toBeVisible();
  expect(screen.getByText("48.2K")).toBeVisible();
  expect(screen.getByText("At API prices")).toBeVisible();
  expect(screen.getByText("$0.84")).toBeVisible();
});

test("leaves cost out when the usage cannot be priced, and usage out before the agent has a session", () => {
  const view = render(panel(session));
  expect(screen.getByText("48.2K")).toBeVisible();
  expect(screen.queryByText("At API prices")).not.toBeInTheDocument();
  view.rerender(panel());
  expect(screen.queryByText("Tokens")).not.toBeInTheDocument();
  expect(screen.getByText("claude-sonnet-5-5")).toBeVisible();
});

test("copies the conversation as Markdown", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  render(panel());
  await userEvent.click(screen.getByRole("button", { name: "More actions" }));
  await userEvent.click(
    screen.getByRole("menuitem", { name: "Copy as Markdown" }),
  );
  expect(writeText).toHaveBeenCalledTimes(1);
  expect(writeText.mock.calls[0][0]).toContain("Claude Code · /work/checkout");
  expect(
    await screen.findByText("Copied the conversation as Markdown."),
  ).toBeVisible();
});
