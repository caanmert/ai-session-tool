import { expect, test } from "vitest";
import type { ChatMessage, ChatState, ChatUpdate } from "./api";
import { applyChatUpdate } from "./chatUpdates";

const message = (id: string, text = id): ChatMessage => ({
  id,
  role: "assistant",
  kind: "text",
  text,
});
const state = (version: number, messages: ChatMessage[]): ChatState =>
  ({ id: "chat", version, status: "running", messages }) as ChatState;
const partial = (
  version: number,
  messages: ChatMessage[],
  first: string,
  count: number,
): ChatUpdate => ({
  version,
  state: state(version, messages),
  partial: true,
  first,
  count,
});

test("a full update reuses messages that did not change", () => {
  const before = state(1, [message("a"), message("b")]);
  const after = applyChatUpdate(before, {
    version: 2,
    state: state(2, [message("a"), message("b", "b more")]),
  })!;
  expect(after.version).toBe(2);
  expect(after.messages[0]).toBe(before.messages[0]);
  expect(after.messages[1]).not.toBe(before.messages[1]);
  expect(after.messages[1].text).toBe("b more");
});

test("no state means nothing changed", () => {
  const before = state(1, [message("a")]);
  expect(applyChatUpdate(before, { version: 1, state: null })).toBe(before);
});

test("a partial update replaces the streamed message and appends new ones", () => {
  const before = state(1, [message("a"), message("b")]);
  const after = applyChatUpdate(
    before,
    partial(2, [message("b", "b more"), message("c")], "a", 3),
  )!;
  expect(after.messages.map((m) => m.text)).toEqual(["a", "b more", "c"]);
  expect(after.messages[0]).toBe(before.messages[0]);
  expect(after.status).toBe("running");
});

test("a partial update with no messages still carries the new status", () => {
  const before = state(1, [message("a")]);
  const update = partial(2, [], "a", 1);
  update.state!.status = "waiting";
  const after = applyChatUpdate(before, update)!;
  expect(after.status).toBe("waiting");
  expect(after.messages).toEqual(before.messages);
});

test("messages trimmed by the backend are dropped", () => {
  const before = state(1, [message("a"), message("b"), message("c")]);
  const after = applyChatUpdate(before, partial(2, [message("d")], "b", 3))!;
  expect(after.messages.map((m) => m.id)).toEqual(["b", "c", "d"]);
});

test("a partial update that does not fit asks for a full read", () => {
  const before = state(1, [message("a")]);
  // The reader missed message "b".
  expect(
    applyChatUpdate(before, partial(3, [message("c")], "a", 3)),
  ).toBeUndefined();
  // The first retained message is unknown to the reader.
  expect(
    applyChatUpdate(before, partial(3, [message("c")], "z", 2)),
  ).toBeUndefined();
  expect(
    applyChatUpdate(undefined, partial(3, [message("c")], "c", 1)),
  ).toBeUndefined();
});
