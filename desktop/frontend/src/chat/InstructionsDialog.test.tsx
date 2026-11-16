import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend } from "../api";
import InstructionsDialog from "./InstructionsDialog";

const file = (agent: string, scope: string, name: string, exists: boolean) => ({
  agent,
  scope,
  name,
  path: scope === "user" ? `/home/${name.slice(2)}` : `/work/app/${name}`,
  exists,
  size: exists ? 1200 : 0,
  editable: scope === "project",
});
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    ProjectInstructions: vi
      .fn()
      .mockResolvedValue([
        file("claude", "project", "CLAUDE.md", true),
        file("claude", "project", ".claude/CLAUDE.md", false),
        file("codex", "project", "AGENTS.md", false),
        file("codex", "user", "~/.codex/AGENTS.md", true),
      ]),
    ReadInstruction: vi.fn(async (_project: string, name: string) =>
      name === "CLAUDE.md"
        ? { name, text: "# Rules\n", exists: true, version: "v1" }
        : { name, text: "", exists: false, version: "" },
    ),
    WriteInstruction: vi.fn(async (_p: string, name: string, text: string) => ({
      name,
      text,
      exists: true,
      version: "v2",
    })),
    RevealInstruction: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("edits and saves a project's instructions with the version read", async () => {
  render(<InstructionsDialog project="/work/app" onClose={vi.fn()} />);
  const editor = await screen.findByLabelText("Contents of CLAUDE.md");
  expect(editor).toHaveValue("# Rules\n");
  const save = screen.getByRole("button", { name: /^Save/ });
  expect(save).toBeDisabled();
  await userEvent.type(editor, "Use tabs.");
  await userEvent.click(save);
  expect(backend.WriteInstruction).toHaveBeenCalledWith(
    "/work/app",
    "CLAUDE.md",
    "# Rules\nUse tabs.",
    "v1",
  );
  expect(await screen.findByText(/Saved CLAUDE.md/)).toBeVisible();
});

test("a stale save is explained, and unsaved edits are not dropped silently", async () => {
  backend.WriteInstruction.mockRejectedValueOnce(
    new Error(
      "CLAUDE.md changed on disk since you opened it; nothing was saved.",
    ),
  );
  const close = vi.fn();
  render(<InstructionsDialog project="/work/app" onClose={close} />);
  const editor = await screen.findByLabelText("Contents of CLAUDE.md");
  await userEvent.type(editor, "x");
  await userEvent.keyboard("{Meta>}s{/Meta}");
  expect(await screen.findByRole("alert")).toHaveTextContent("changed on disk");
  // Choosing another file with edits asks first.
  await userEvent.click(
    screen.getByRole("button", { name: /AGENTS\.md not created/ }),
  );
  expect(screen.getByText(/unsaved changes to CLAUDE.md/)).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Keep editing" }));
  expect(editor).toHaveValue("# Rules\nx");
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.click(
    screen.getByRole("button", { name: "Discard changes" }),
  );
  expect(close).toHaveBeenCalled();
});

test("a new project file is created on save; user files open in Finder", async () => {
  render(<InstructionsDialog project="/work/app" onClose={vi.fn()} />);
  await screen.findByLabelText("Contents of CLAUDE.md");
  const nav = screen.getByRole("navigation", { name: "Instructions files" });
  await userEvent.click(
    within(nav).getByRole("button", { name: /^AGENTS\.md/ }),
  );
  expect(
    await screen.findByText(/new file, created when you save/),
  ).toBeVisible();
  await userEvent.click(
    within(nav).getByRole("button", { name: /~\/\.codex\/AGENTS\.md/ }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Show in Finder" }));
  await waitFor(() =>
    expect(backend.RevealInstruction).toHaveBeenCalledWith(
      "/work/app",
      "~/.codex/AGENTS.md",
    ),
  );
  expect(screen.queryByRole("button", { name: /^Save/ })).toBeNull();
});
