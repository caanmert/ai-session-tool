import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, ProjectRunState } from "../api";
import RunPanel, { runOutcome } from "./RunPanel";

let backend: Record<string, ReturnType<typeof vi.fn>>;
let state: ProjectRunState;
beforeEach(() => {
  state = {
    command: "npm run test",
    running: true,
    exitCode: 0,
    output: "",
    startedAt: "2026-11-17T10:00:00Z",
  };
  backend = {
    ProjectCommands: vi.fn().mockResolvedValue([
      { label: "test", command: "npm run test", source: "package.json" },
      { label: "build", command: "npm run build", source: "package.json" },
    ]),
    ProjectRun: vi
      .fn()
      .mockRejectedValue(new Error("nothing has run here yet")),
    StartProjectCommand: vi.fn(async (_id: string, command: string) => {
      backend.ProjectRun.mockImplementation(async () => state);
      return { ...state, command };
    }),
    StopProjectCommand: vi.fn(async () => {
      state = { ...state, running: false, stopped: true, exitCode: -1 };
    }),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("runs nothing until Run is pressed, then follows the output", async () => {
  render(<RunPanel chatId="c1" />);
  const run = screen.getByRole("button", { name: "Run" });
  expect(run).toBeDisabled();
  await userEvent.click(await screen.findByRole("button", { name: "test" }));
  expect(screen.getByLabelText(/Command to run/)).toHaveValue("npm run test");
  expect(backend.StartProjectCommand).not.toHaveBeenCalled();
  await userEvent.click(run);
  expect(backend.StartProjectCommand).toHaveBeenCalledWith(
    "c1",
    "npm run test",
  );
  expect(await screen.findByRole("button", { name: "Stop" })).toBeVisible();
  state = { ...state, output: "✓ 12 tests\n", running: false, endedAt: "x" };
  expect(await screen.findByText(/12 tests/)).toBeVisible();
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("Finished"),
  );
  expect(screen.getByRole("button", { name: "Run" })).toBeEnabled();
});

test("a typed command runs with Enter and can be stopped", async () => {
  render(<RunPanel chatId="c1" />);
  await userEvent.type(
    screen.getByLabelText(/Command to run/),
    "sleep 30{Enter}",
  );
  expect(backend.StartProjectCommand).toHaveBeenCalledWith("c1", "sleep 30");
  await userEvent.click(await screen.findByRole("button", { name: "Stop" }));
  expect(backend.StopProjectCommand).toHaveBeenCalledWith("c1");
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("Stopped"),
  );
});

test("describes how a run ended", () => {
  const base = { ...state, running: false };
  expect(runOutcome({ ...base, exitCode: 0 })).toBe("Finished");
  expect(runOutcome({ ...base, exitCode: 3 })).toBe("Failed with exit code 3");
  expect(runOutcome({ ...base, exitCode: -1, stopped: true })).toBe("Stopped");
  expect(runOutcome({ ...base, running: true })).toBe("Running…");
});
