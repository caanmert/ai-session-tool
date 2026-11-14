import type { ChatState } from "../api";

const busy = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);

export type Attention = { waiting: number; busy: number };

/**
 * Decide what the Dock should show after the open conversations changed.
 * Returns undefined when nothing relevant changed. `alert` asks for a bounce:
 * a conversation started waiting for the user, or a response ended.
 */
export function attentionChange(before: Attention, chats: ChatState[]) {
  const now: Attention = {
    waiting: chats.filter((chat) => chat.status === "waiting").length,
    busy: chats.filter((chat) => busy(chat.status)).length,
  };
  if (now.waiting === before.waiting && now.busy >= before.busy)
    return { now, update: undefined };
  return {
    now,
    update: {
      count: now.waiting,
      alert: now.waiting > before.waiting || now.busy < before.busy,
    },
  };
}
