import type { ChatMessage, ChatState, ChatUpdate } from "./api";

const sameMessage = (a: ChatMessage, b: ChatMessage) =>
  a.id === b.id &&
  a.text === b.text &&
  a.status === b.status &&
  a.model === b.model &&
  a.name === b.name &&
  a.role === b.role &&
  a.kind === b.kind;

/**
 * Apply a backend update to the conversation on screen.
 *
 * Unchanged messages keep their object identity, so memoized rows skip
 * rendering and only the message being streamed is drawn again.
 *
 * Returns undefined when a partial update does not fit the current state (an
 * update was missed). The caller then reads the whole conversation again.
 */
export function applyChatUpdate(
  current: ChatState | undefined,
  update: ChatUpdate,
): ChatState | undefined {
  const next = update.state;
  if (!next) return current;
  const known = new Map(current?.messages.map((m) => [m.id, m]));
  const keep = (message: ChatMessage) => {
    const old = known.get(message.id);
    return old && sameMessage(old, message) ? old : message;
  };
  if (!update.partial) return { ...next, messages: next.messages.map(keep) };
  if (!current) return undefined;

  // Older messages are trimmed from the front once the display limit is hit.
  let messages = current.messages;
  if (update.first) {
    const start = messages.findIndex((m) => m.id === update.first);
    if (start > 0) messages = messages.slice(start);
  } else messages = [];
  const changed = new Map(next.messages.map((m) => [m.id, m]));
  messages = messages.map((m) => {
    const replacement = changed.get(m.id);
    if (!replacement) return m;
    changed.delete(m.id);
    return keep(replacement);
  });
  // Whatever is left is new, and new messages are only ever added at the end.
  if (changed.size)
    messages = [...messages, ...next.messages.filter((m) => changed.has(m.id))];
  if (
    messages.length !== (update.count ?? 0) ||
    (messages[0]?.id ?? "") !== (update.first ?? "")
  )
    return undefined;
  return { ...next, messages };
}
