import { expect, test } from "vitest";
import type { ChatState } from "../api";
import {
  attentionChange,
  attentionNotices,
  notificationsEnabled,
  setNotificationsEnabled,
} from "./attention";

const chats = (...statuses: string[]) =>
  statuses.map((status, i) => ({ id: String(i), status })) as ChatState[];

test("a conversation that starts waiting sets the badge and asks for an alert", () => {
  const { now, update } = attentionChange(
    { waiting: 0, busy: 1 },
    chats("waiting", "idle"),
  );
  expect(now).toEqual({ waiting: 1, busy: 1 });
  expect(update).toEqual({ count: 1, alert: true });
});

test("answering clears the badge without an alert", () => {
  expect(
    attentionChange({ waiting: 1, busy: 1 }, chats("running")).update,
  ).toEqual({ count: 0, alert: false });
});

test("a response that ends asks for an alert", () => {
  expect(
    attentionChange({ waiting: 0, busy: 2 }, chats("running", "idle")).update,
  ).toEqual({ count: 0, alert: true });
});

test("starting work or unrelated changes do nothing", () => {
  expect(
    attentionChange({ waiting: 0, busy: 0 }, chats("running")).update,
  ).toBeUndefined();
  expect(
    attentionChange({ waiting: 1, busy: 1 }, chats("waiting", "idle")).update,
  ).toBeUndefined();
});

const named = (id: string, status: string, patch: Partial<ChatState> = {}) =>
  ({
    id,
    status,
    tool: "claude",
    title: `Task ${id}`,
    approvals: [],
    ...patch,
  }) as ChatState;

test("notices cover waiting and finishing, not starting or stopping by hand", () => {
  const before = new Map([
    ["a", "running"],
    ["b", "running"],
    ["c", "running"],
    ["d", "idle"],
    ["e", "running"],
  ]);
  const { now, notices } = attentionNotices(before, [
    named("a", "waiting", {
      approvals: [{ id: "r", title: "Run", detail: "npm test", questions: [] }],
    }),
    named("b", "idle", { tool: "codex" }),
    named("c", "stopped"),
    named("d", "running"),
    named("e", "error"),
    named("new", "waiting"),
  ]);
  expect(notices).toEqual([
    { title: "Task a", body: "Claude Code: needs approval" },
    { title: "Task b", body: "Codex finished" },
    { title: "Task e", body: "Claude Code stopped with an error" },
  ]);
  expect(now.get("new")).toBe("waiting");
  // Nothing changed since: nothing to say.
  expect(attentionNotices(now, [named("a", "waiting")]).notices).toEqual([]);
});

test("notifications are on by default and the choice is remembered", () => {
  window.localStorage.clear();
  expect(notificationsEnabled()).toBe(true);
  expect(setNotificationsEnabled(false)).toBe(true);
  expect(notificationsEnabled()).toBe(false);
  setNotificationsEnabled(true);
  expect(notificationsEnabled()).toBe(true);
});
