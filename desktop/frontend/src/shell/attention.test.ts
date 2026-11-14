import { expect, test } from "vitest";
import type { ChatState } from "../api";
import { attentionChange } from "./attention";

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
