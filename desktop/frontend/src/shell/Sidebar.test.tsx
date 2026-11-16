import { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import type { ChatState, Snapshot } from "../api";
import Sidebar from "./Sidebar";

const snapshot = {
  sessions: [],
  projects: [],
  tools: [
    { name: "codex", available: true, path: "/bin/codex" },
    { name: "claude", available: true, path: "/bin/claude" },
  ],
} as unknown as Snapshot;
const chats = [
  {
    id: "a",
    title: "Fix webhook retries",
    tool: "codex",
    project: "/api",
    status: "idle",
  },
  {
    id: "b",
    title: "Review checkout",
    tool: "claude",
    project: "/api",
    status: "waiting",
  },
  {
    id: "c",
    title: "Docs pass",
    tool: "codex",
    project: "/docs",
    status: "running",
  },
] as ChatState[];

function Shell({ rail = false, select = vi.fn() }) {
  const [folded, setFolded] = useState<string[]>([]);
  const [active, setActive] = useState("a");
  return (
    <Sidebar
      view="chat"
      snapshot={snapshot}
      chats={chats}
      activeChat={active}
      closingChat=""
      newButton={{ current: null }}
      canStart
      rail={rail}
      folded={folded}
      onFold={(key) =>
        setFolded((keys) =>
          keys.includes(key) ? keys.filter((k) => k !== key) : [...keys, key],
        )
      }
      onNew={vi.fn()}
      onHistory={vi.fn()}
      onRuns={vi.fn()}
      onSaved={vi.fn()}
      onSettings={vi.fn()}
      onBoard={vi.fn()}
      onIssues={vi.fn()}
      onUsage={vi.fn()}
      onSelectChat={(id) => {
        select(id);
        setActive(id);
      }}
      onCloseChat={vi.fn()}
    />
  );
}

test("sections fold, say what they hold, and keep the open conversation in view", async () => {
  render(<Shell />);
  const agents = screen.getByRole("button", { name: "Agents" });
  expect(agents).toHaveAttribute("aria-expanded", "true");
  await userEvent.click(agents);
  expect(
    screen.getByRole("button", { name: "Agents, folded, 2" }),
  ).toHaveAttribute("aria-expanded", "false");
  // The agent rows are folded away.
  expect(screen.queryByTitle("/bin/codex")).toBeNull();

  const api = screen.getByRole("button", { name: "api" });
  await userEvent.click(api);
  const folded = screen.getByRole("button", {
    name: "api, folded, 2, 1 need you",
  });
  expect(within(folded).getByText("2")).toBeVisible();
  // The open conversation stays visible; the other one is folded away.
  expect(
    screen.getByRole("button", { name: /^Fix webhook retries/ }),
  ).toBeVisible();
  expect(screen.queryByRole("button", { name: /^Review checkout/ })).toBeNull();
  // Arrow keys move only through what is shown.
  screen.getByRole("button", { name: /^Fix webhook retries/ }).focus();
  await userEvent.keyboard("{ArrowDown}");
  expect(screen.getByRole("button", { name: /^Docs pass/ })).toHaveFocus();
  await userEvent.click(folded);
  expect(
    screen.getByRole("button", { name: /^Review checkout/ }),
  ).toBeVisible();
});

test("the narrow sidebar keeps every page, agent and conversation reachable", async () => {
  const select = vi.fn();
  render(<Shell rail select={select} />);
  const sidebar = screen.getByRole("complementary", { name: "Workspace" });
  expect(
    within(sidebar).getByRole("button", { name: "New task" }),
  ).toHaveAttribute("title", "New task (⌘N)");
  for (const name of [
    "History",
    "Issues & PRs",
    "Usage limits",
    "Parallel runs",
    "Saved workspace",
    "Settings",
  ])
    expect(within(sidebar).getByRole("button", { name })).toBeVisible();
  // A conversation waiting for you is counted on the board.
  expect(
    within(sidebar).getByRole("button", { name: "Agents board, 1 need you" }),
  ).toHaveTextContent("1");
  expect(
    within(sidebar).getByRole("button", {
      name: "Codex: Ready. Open usage limits",
    }),
  ).toBeVisible();
  const waiting = within(sidebar).getByRole("button", {
    name: "Review checkout, Claude, Needs your input",
  });
  await userEvent.click(waiting);
  expect(select).toHaveBeenCalledWith("b");
  expect(waiting).toHaveAttribute("aria-pressed", "true");
  await userEvent.keyboard("{ArrowDown}");
  expect(select).toHaveBeenLastCalledWith("c");
});
