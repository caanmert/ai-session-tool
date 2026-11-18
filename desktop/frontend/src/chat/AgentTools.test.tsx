import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend } from "../api";
import AgentToolsSection from "./AgentTools";

let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    AgentTools: vi.fn().mockResolvedValue({
      tool: "codex",
      servers: [
        {
          name: "node_repl",
          state: "connected",
          detail: "2 tools",
          tools: ["eval", "js"],
        },
        { name: "linear", state: "needs-auth", detail: "Needs sign-in" },
        { name: "idle", state: "unknown", detail: "No tools reported" },
      ],
    }),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("asks the agent only when asked, problems first", async () => {
  render(<AgentToolsSection chatId="chat" tool="codex" />);
  expect(backend.AgentTools).not.toHaveBeenCalled();
  expect(
    screen.getByText(
      "The MCP servers Codex can use here, and whether they work.",
    ),
  ).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Check tools" }));
  expect(backend.AgentTools).toHaveBeenCalledWith("chat");
  expect(await screen.findByText("3 servers, 1 not working.")).toBeVisible();
  const rows = within(
    screen.getByRole("region", { name: "Agent tools" }),
  ).getAllByRole("listitem");
  expect(rows.map((row) => row.firstChild?.textContent)).toEqual([
    "linear",
    "node_repl",
    "idle",
  ]);
  expect(rows[0]).toHaveTextContent("Needs sign-in");
  await userEvent.click(within(rows[1]).getByText("Tools"));
  expect(within(rows[1]).getByText("eval, js")).toBeVisible();
  expect(screen.getByRole("button", { name: "Check again" })).toBeEnabled();
});

test("says when there are none, or the check failed", async () => {
  backend.AgentTools = vi
    .fn()
    .mockResolvedValueOnce({ tool: "claude", servers: [] })
    .mockRejectedValueOnce(
      new Error("Claude Code did not list its MCP servers in time"),
    );
  render(<AgentToolsSection chatId="chat" tool="claude" />);
  await userEvent.click(screen.getByRole("button", { name: "Check tools" }));
  expect(
    await screen.findByText("Claude Code has no MCP servers in this project."),
  ).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Check again" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("in time");
});
