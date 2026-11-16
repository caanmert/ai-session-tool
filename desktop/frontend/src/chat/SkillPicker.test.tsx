import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { AgentSkill, Backend, Project, Tool } from "../api";
import SkillsSection from "../settings/SkillsSection";
import { matchSkills, skillAt, skillToken } from "./SkillPicker";

test("a skill is being typed after / (and $ for Codex) at a word start", () => {
  expect(skillAt("/rev", 4, "claude")).toEqual({ start: 0, query: "rev" });
  expect(skillAt("please /", 8, "claude")).toEqual({ start: 7, query: "" });
  expect(skillAt("src/app", 7, "claude")).toBeNull();
  expect(skillAt("/review now", 11, "claude")).toBeNull();
  expect(skillAt("$pro", 4, "claude")).toBeNull();
  expect(skillAt("$pro", 4, "codex")).toEqual({ start: 0, query: "pro" });
  expect(skillToken("codex", "pdf:pdf")).toBe("$pdf:pdf");
  expect(skillToken("claude", "review")).toBe("/review");
});

test("names that match come before descriptions that do", () => {
  const skills: AgentSkill[] = [
    { name: "docs", description: "Write a review of the docs" },
    { name: "review", description: "Review the diff" },
    { name: "deploy", description: "Ship it" },
  ];
  expect(matchSkills(skills, "REV").map((s) => s.name)).toEqual([
    "review",
    "docs",
  ]);
  expect(matchSkills(skills, "")).toHaveLength(3);
});

let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    AgentSkills: vi.fn(async (tool: string) =>
      tool === "claude"
        ? {
            tool,
            skills: [
              {
                name: "review",
                description: "Review the diff",
                hint: "[files]",
              },
            ],
          }
        : {
            tool,
            skills: [],
            reason: "Codex could not list its skills: offline",
          },
    ),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("Settings lists each agent's skills for a project and reads again on request", async () => {
  const tools: Tool[] = [
    { name: "claude", available: true, path: "/bin/claude" },
    { name: "codex", available: true, path: "/bin/codex" },
  ];
  const projects = [
    { path: "/work/app", name: "app", available: true },
  ] as Project[];
  render(<SkillsSection tools={tools} projects={projects} project="" />);
  const claude = await screen.findByRole("group", {
    name: "Claude Code skills",
  });
  expect(await within(claude).findByText("review")).toBeVisible();
  expect(within(claude).getByText("1 skill · use with /name")).toBeVisible();
  const codex = screen.getByRole("group", { name: "Codex skills" });
  expect(await within(codex).findByText(/offline/)).toBeVisible();
  expect(backend.AgentSkills).toHaveBeenCalledWith(
    "claude",
    "/work/app",
    false,
  );
  await userEvent.click(screen.getByRole("button", { name: /Read again/ }));
  await waitFor(() =>
    expect(backend.AgentSkills).toHaveBeenCalledWith(
      "claude",
      "/work/app",
      true,
    ),
  );
});
