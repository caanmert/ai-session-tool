import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import type { Backend } from "../api";
import SavedPromptsSection from "./SavedPromptsSection";

let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    SavedPrompts: vi.fn().mockResolvedValue([
      {
        id: "starter-review",
        name: "Review",
        text: "Review {changed files}",
      },
    ]),
    SetSavedPrompts: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

it("adds, edits and removes saved prompts", async () => {
  render(<SavedPromptsSection />);
  expect(await screen.findByText("Review")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /New prompt/ }));
  await userEvent.type(screen.getByLabelText("Name"), "Lint");
  await userEvent.type(screen.getByLabelText("Text"), "Run the linter");
  await userEvent.click(screen.getByRole("button", { name: "Save prompt" }));
  const added = backend.SetSavedPrompts.mock.calls[0][0];
  expect(added.map((p: { name: string }) => p.name)).toEqual([
    "Review",
    "Lint",
  ]);
  expect(await screen.findByText("Lint")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Edit Review" }));
  const name = screen.getByLabelText("Name");
  await userEvent.clear(name);
  await userEvent.type(name, "Review for bugs");
  await userEvent.click(screen.getByRole("button", { name: "Save prompt" }));
  expect(await screen.findByText("Review for bugs")).toBeVisible();
  await userEvent.click(
    screen.getByRole("button", { name: "Remove Review for bugs" }),
  );
  await waitFor(() =>
    expect(backend.SetSavedPrompts).toHaveBeenLastCalledWith([
      expect.objectContaining({ name: "Lint" }),
    ]),
  );
});

it("explains a refused save and keeps the form", async () => {
  backend.SetSavedPrompts.mockRejectedValueOnce(
    new Error("a saved prompt needs a name and text"),
  );
  render(<SavedPromptsSection />);
  await screen.findByText("Review");
  await userEvent.click(screen.getByRole("button", { name: "Edit Review" }));
  await userEvent.click(screen.getByRole("button", { name: "Save prompt" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("needs a name");
  expect(screen.getByLabelText("Name")).toBeVisible();
});
