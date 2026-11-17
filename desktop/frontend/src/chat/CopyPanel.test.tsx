import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import type { Backend, ChatState } from "../api";
import CopyPanel from "./CopyPanel";

const chat = {
  id: "c1",
  tool: "codex",
  project: "/data/copies/1a2b/app",
  copy: {
    project: "/work/app",
    base: "abcdef123456",
    branch: "ais/1a2b",
    worktree: "/data/copies/1a2b/app",
  },
} as ChatState;
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    CopyChanges: vi.fn().mockResolvedValue({
      root: "",
      base: "",
      hash: "",
      diff: "",
      warnings: [],
      files: [
        { path: "a.ts", patch: "@@ -1 +1 @@\n-a\n+b\n" },
        { path: "b.ts", patch: "@@ -0,0 +1 @@\n+c\n" },
      ],
    }),
    BringBackCopy: vi.fn().mockResolvedValue(["a.ts", "b.ts"]),
    RemoveCopy: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

it("brings the copy's changes back after listing them", async () => {
  const onDone = vi.fn();
  render(
    <CopyPanel chat={chat} busy={false} onDone={onDone} onRemoved={vi.fn()} />,
  );
  expect(screen.getByText("ais/1a2b")).toBeVisible();
  await userEvent.click(
    screen.getByRole("button", { name: "Bring changes back…" }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Bring 2 files back into the project?",
  });
  expect(within(dialog).getByText("a.ts")).toBeVisible();
  expect(backend.BringBackCopy).not.toHaveBeenCalled();
  await userEvent.click(
    within(dialog).getByRole("button", { name: "Bring them back" }),
  );
  expect(backend.BringBackCopy).toHaveBeenCalledWith("c1");
  await waitFor(() =>
    expect(onDone).toHaveBeenCalledWith(
      "Brought 2 files back into the project, uncommitted.",
    ),
  );
});

it("counts every file to bring back when the view left some out", async () => {
  backend.CopyChanges.mockResolvedValueOnce({
    root: "",
    base: "",
    hash: "",
    diff: "",
    warnings: ["Only the first 1 of 30 changed files are shown."],
    files: [{ path: "a.ts", patch: "@@ -1 +1 @@\n-a\n+b\n" }],
    total: 30,
    partial: true,
  });
  render(
    <CopyPanel chat={chat} busy={false} onDone={vi.fn()} onRemoved={vi.fn()} />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Bring changes back…" }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Bring 30 files back into the project?",
  });
  expect(within(dialog).getByText("a.ts")).toBeVisible();
  expect(within(dialog).getByText("and 29 more")).toBeVisible();
});

it("asks again before discarding changes that were not brought back", async () => {
  backend.RemoveCopy.mockRejectedValueOnce(
    new Error(
      "the copy has changes that were not brought back (2 files); bring them back or choose to discard them",
    ),
  );
  const onRemoved = vi.fn();
  render(
    <CopyPanel
      chat={chat}
      busy={false}
      onDone={vi.fn()}
      onRemoved={onRemoved}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Remove copy…" }));
  await userEvent.click(
    (
      await screen.findByRole("dialog", { name: "Remove this copy?" })
    ).querySelector("button.danger, button.primary")! as HTMLElement,
  );
  const discard = await screen.findByRole("dialog", {
    name: "Discard the copy's changes?",
  });
  expect(backend.RemoveCopy).toHaveBeenCalledWith("c1", false);
  expect(onRemoved).not.toHaveBeenCalled();
  // The destructive choice is never the default.
  expect(
    within(discard).getByRole("button", { name: "Discard and remove" }),
  ).not.toHaveFocus();
  await userEvent.click(
    within(discard).getByRole("button", { name: "Discard and remove" }),
  );
  expect(backend.RemoveCopy).toHaveBeenLastCalledWith("c1", true);
  await waitFor(() => expect(onRemoved).toHaveBeenCalled());
});

it("waits while the agent works", () => {
  render(<CopyPanel chat={chat} busy onDone={vi.fn()} onRemoved={vi.fn()} />);
  expect(
    screen.getByRole("button", { name: "Bring changes back…" }),
  ).toBeDisabled();
  expect(screen.getByRole("button", { name: "Remove copy…" })).toBeDisabled();
});
