import { toolLabel, type ChatState } from "../api";
import { conversationStatus } from "../chat/conversation";

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

export type Notice = {
  title: string;
  body: string;
  /** The conversation to open when the banner is clicked. */
  chatId: string;
  /** Waiting for an answer, or stopped with an error; not just finished. */
  needsYou: boolean;
};

/**
 * What is worth telling someone who is not looking at the app: a conversation
 * started waiting for them, or a response ended on its own. A response the
 * user stopped, and conversations seen for the first time, are not news.
 */
export function attentionNotices(
  before: Map<string, string>,
  chats: ChatState[],
) {
  const now = new Map(chats.map((chat) => [chat.id, chat.status]));
  const notices: Notice[] = [];
  for (const chat of chats) {
    const was = before.get(chat.id);
    if (was === undefined || was === chat.status) continue;
    const title = chat.title || "New conversation";
    const agent = toolLabel(chat.tool);
    if (chat.status === "waiting")
      notices.push({
        title,
        body: `${agent}: ${conversationStatus(chat).toLowerCase()}`,
        chatId: chat.id,
        needsYou: true,
      });
    else if (busy(was) && chat.status === "idle")
      notices.push({
        title,
        body: `${agent} finished`,
        chatId: chat.id,
        needsYou: false,
      });
    else if (busy(was) && chat.status === "error")
      notices.push({
        title,
        body: `${agent} stopped with an error`,
        chatId: chat.id,
        needsYou: true,
      });
  }
  return { now, notices };
}

const key = "ais:notifications:v1";

/** Banner notifications are on unless the user turned them off. */
export function notificationsEnabled() {
  try {
    return window.localStorage.getItem(key) !== "off";
  } catch {
    return true;
  }
}

/** Returns false when the choice could not be saved. */
export function setNotificationsEnabled(on: boolean) {
  try {
    if (on) window.localStorage.removeItem(key);
    else window.localStorage.setItem(key, "off");
    return true;
  } catch {
    return false;
  }
}

const needsKey = "ais:notifications-needs:v1";

/** Banners only when an agent needs the user, not when one just finishes. */
export function notifyOnlyWhenNeeded() {
  try {
    return window.localStorage.getItem(needsKey) === "on";
  } catch {
    return false;
  }
}

/** Returns false when the choice could not be saved. */
export function setNotifyOnlyWhenNeeded(on: boolean) {
  try {
    if (on) window.localStorage.setItem(needsKey, "on");
    else window.localStorage.removeItem(needsKey);
    return true;
  } catch {
    return false;
  }
}

/** The banners worth showing under the user's choices. */
export function wantedNotices(notices: Notice[]) {
  if (!notificationsEnabled()) return [];
  return notifyOnlyWhenNeeded() ? notices.filter((n) => n.needsYou) : notices;
}
