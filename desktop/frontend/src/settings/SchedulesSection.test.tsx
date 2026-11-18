import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, Schedule } from "../api";
import SchedulesSection, { lastText, whenText } from "./SchedulesSection";

const task: Schedule = {
  id: "t1",
  name: "Evening review",
  project: "/work/shop",
  tool: "codex",
  prompt: "Review today's changes",
  mode: "plan",
  days: "weekdays",
  time: "18:00",
  enabled: false,
  lastAt: "2026-11-17T15:00:00Z",
  lastResult: "missed",
  lastNote: "ais was not open.",
};
const tools = [
  { name: "codex", available: true },
  { name: "claude", available: false },
] as never;
const projects = [
  {
    path: "/work/shop",
    name: "shop",
    sessions: 1,
    available: true,
    recent: true,
  },
];
let backend: Record<string, ReturnType<typeof vi.fn>>;
let list: Schedule[];
beforeEach(() => {
  list = [structuredClone(task)];
  backend = {
    Schedules: vi.fn(async () => structuredClone(list)),
    SaveSchedule: vi.fn(async (next: Schedule) => {
      const saved = { ...next, id: next.id || "t2", enabled: false };
      list = [...list.filter((t) => t.id !== saved.id), saved];
      return saved;
    }),
    SetScheduleEnabled: vi.fn(async (id: string, on: boolean) => {
      list = list.map((t) =>
        t.id === id
          ? { ...t, enabled: on, next: on ? "2026-11-18T15:00:00Z" : undefined }
          : t,
      );
    }),
    DeleteSchedule: vi.fn(async (id: string) => {
      list = list.filter((t) => t.id !== id);
    }),
    RunSchedule: vi.fn().mockResolvedValue({ id: "chat-9" }),
    PickProject: vi.fn().mockResolvedValue("/work/other"),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("reads a task's time and last run in words", () => {
  expect(whenText(task)).toBe("Weekdays at 18:00");
  expect(whenText({ days: "sun", time: "09:30" })).toBe("Sundays at 09:30");
  expect(lastText(task)).toMatch(/^Missed .+: ais was not open\.$/);
  expect(lastText({ ...task, lastResult: undefined })).toBe("");
});

test("a task is turned on only after a confirmation", async () => {
  render(<SchedulesSection tools={tools} projects={projects} />);
  const toggle = await screen.findByRole("switch", {
    name: "Run Evening review on schedule",
  });
  expect(
    screen.getByText("Weekdays at 18:00 · Codex · shop · Plan"),
  ).toBeVisible();
  expect(screen.getByText(/^Missed .*ais was not open/)).toBeVisible();
  await userEvent.click(toggle);
  const dialog = screen.getByRole("dialog", {
    name: /Turn on “Evening review”/,
  });
  expect(dialog).toHaveTextContent(
    "it will start Codex on its own in shop and send the task's message, in Plan mode",
  );
  // The safe choice has the focus.
  expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
  expect(backend.SetScheduleEnabled).not.toHaveBeenCalled();
  await userEvent.click(
    within(dialog).getByRole("button", { name: "Turn on" }),
  );
  await waitFor(() =>
    expect(backend.SetScheduleEnabled).toHaveBeenCalledWith("t1", true),
  );
  expect(await screen.findByText(/^Next: /)).toBeVisible();
  // Off needs no confirmation.
  await userEvent.click(
    screen.getByRole("switch", { name: "Run Evening review on schedule" }),
  );
  await waitFor(() =>
    expect(backend.SetScheduleEnabled).toHaveBeenLastCalledWith("t1", false),
  );
});

test("creates a task off, runs one now, and removes one after asking", async () => {
  const onOpenChat = vi.fn();
  render(
    <SchedulesSection
      tools={tools}
      projects={projects}
      onOpenChat={onOpenChat}
    />,
  );
  await screen.findByText("Evening review");
  await userEvent.click(screen.getByRole("button", { name: "New task" }));
  // Claude Code is not installed, so it cannot be chosen.
  expect(screen.getByRole("option", { name: "Claude Code" })).toBeDisabled();
  const save = screen.getByRole("button", { name: "Save task" });
  expect(save).toBeDisabled();
  await userEvent.type(screen.getByRole("textbox", { name: "Name" }), "Deps");
  await userEvent.type(
    screen.getByRole("textbox", { name: "Message" }),
    "Check for dependency updates",
  );
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Days" }),
    "mon",
  );
  await userEvent.click(screen.getByRole("button", { name: "Choose…" }));
  await userEvent.click(save);
  await waitFor(() =>
    expect(backend.SaveSchedule).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Deps",
        prompt: "Check for dependency updates",
        project: "/work/other",
        tool: "codex",
        mode: "plan",
        days: "mon",
        time: "18:00",
        enabled: false,
      }),
    ),
  );
  expect(
    await screen.findByText("Mondays at 18:00 · Codex · other · Plan"),
  ).toBeVisible();
  await userEvent.click(
    screen.getByRole("button", { name: "Run Evening review now" }),
  );
  await waitFor(() => expect(onOpenChat).toHaveBeenCalledWith("chat-9"));
  expect(backend.RunSchedule).toHaveBeenCalledWith("t1");
  await userEvent.click(screen.getByRole("button", { name: "Remove Deps" }));
  const dialog = screen.getByRole("dialog", { name: "Remove “Deps”?" });
  expect(within(dialog).getByRole("button", { name: "Keep" })).toHaveFocus();
  await userEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
  await waitFor(() =>
    expect(backend.DeleteSchedule).toHaveBeenCalledWith("t2"),
  );
  await waitFor(() =>
    expect(screen.queryByText("Deps")).not.toBeInTheDocument(),
  );
});

test("opens the conversation of the latest run", async () => {
  list = [
    {
      ...task,
      enabled: true,
      lastResult: "started",
      lastNote: "",
      lastChat: "chat-1",
    },
  ];
  const onOpenChat = vi.fn();
  render(
    <SchedulesSection
      tools={tools}
      projects={projects}
      onOpenChat={onOpenChat}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Open conversation" }),
  );
  expect(onOpenChat).toHaveBeenCalledWith("chat-1");
});
